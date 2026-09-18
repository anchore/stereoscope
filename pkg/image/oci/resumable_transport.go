package oci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anchore/stereoscope/internal/log"
)

const (
	// resumeMinSize is the response size above which a body is worth wrapping. below roughly this
	// size a response arrives in too few segments for a mid-body drop to be a realistic failure,
	// and a request that fails before its body starts flowing is already retried by
	// go-containerregistry. it is a size predicate rather than a "blobs only" one, so an unusually
	// large manifest is wrapped too; gating on a /blobs/ path segment was rejected because the URL
	// actually read is the post-redirect CDN one, whose shape is the CDN's business.
	resumeMinSize = 1024 * 1024

	// maxConsecutiveStalls is how many attempts in a row may fail to deliver meaningful progress
	// before the read is abandoned. progress replenishes it, so a drop in the middle of an
	// otherwise healthy transfer costs nothing.
	maxConsecutiveStalls = 5

	// maxTotalResumes bounds the reopens for one body however much progress each one makes.
	// maxConsecutiveStalls cannot do this alone: any minResumeProgress delivered replenishes it, so
	// a path dropping every few kilobytes would reconnect indefinitely while technically advancing,
	// which on a multi-gigabyte layer reads to an operator as a hang rather than a failure.
	//
	// this is a safety net for the occasional drop, not a recovery mechanism for a broken path: a
	// pull needing more reopens than this should fail, and loudly, as it did before this transport
	// existed. it stays a comfortable multiple of maxConsecutiveStalls, since a single drop may
	// legitimately spend that whole budget on its own.
	maxTotalResumes = 20

	// minResumeProgress is how much must arrive to count as progress. any n > 0 is not enough: a
	// server delivering one byte per connection would otherwise replenish the budget for ever. the
	// surplus above it is discarded rather than banked, so each replenishment is earned afresh.
	minResumeProgress = 64 * 1024

	// reopenHeaderTimeout bounds how long a reopen may take to produce response headers. an
	// endpoint that accepts the connection and then falls silent is otherwise ended only by the
	// pull's context, which callers do not always give a deadline. it covers the headers only, so
	// a body that is arriving slowly is never on a clock.
	reopenHeaderTimeout = 30 * time.Second

	// stallBackoff is multiplied by the consecutive stall count to space out repeated attempts. the
	// first attempt after a drop is made immediately, so an isolated drop costs no delay.
	stallBackoff = time.Second
)

// errNoRangeSupport marks a server that will not resume however often it is asked: it answered the
// range request with a status that settles the matter -- a plain 200, a 416, a redirect -- or with
// a 206 whose Content-Range is unusable or starts at the wrong byte, or with a body in a different
// content coding from the stream so far. a server that was merely busy is errResumeUnavailable,
// and one that refused to authorize the request is errCredentialsRejected.
var errNoRangeSupport = errors.New("server does not support resuming with range requests")

// errObjectChanged marks a resume whose Content-Range reports a different total length from the
// one the read began with, which is direct evidence the object is not the one we started.
var errObjectChanged = errors.New("the object changed while it was being read")

// errResumeUnavailable marks a reopen the server could not serve just now: it was rate limited or
// briefly broken. this is the condition resuming exists for -- a CDN edge unhealthy enough to
// sever a multi-gigabyte stream is exactly the one liable to answer the reopen with a 503 -- so it
// spends a stall and goes round again rather than discarding everything already delivered.
var errResumeUnavailable = errors.New("the server could not serve the resumed range just now")

// errCredentialsRejected marks a reopen the server refused to authorize. it is permanent here on
// purpose: reopen reissues the original request below go-containerregistry's auth transport, so
// every attempt would carry the same expired token or the same stale pre-signed URL and earn the
// same refusal. retrying would only turn one honest failure into ten. mending it properly means
// giving this transport a way to re-authorize, which is a larger change than resuming.
var errCredentialsRejected = errors.New("the server rejected the credentials on the resumed range request")

// resumableTransport wraps a RoundTripper so that a large GET whose body fails partway through is
// transparently resumed with a Range request from the byte already delivered.
//
// registries fronted by a CDN close the connection mid-blob on multi-gigabyte layers.
// go-containerregistry retries at the request level only, so once bytes are flowing a dropped
// connection surfaces as an unexpected EOF and the entire layer is discarded, however much of it
// had already arrived.
//
// this sits below go-containerregistry's verification wrapper, so for a blob the digest and size
// of the reassembled stream are still checked end to end: a resume that splices in the wrong bytes
// fails the layer exactly as a corrupt single-shot download would. that backstop is the blob
// path's and not a universal one -- a manifest fetched by tag is not digest-verified -- which is
// why the offset, length and coding of every resumed response are checked here as well.
//
// termination is two budgets and no more: consecutive attempts that deliver no meaningful
// progress, and reopens in total. an earlier revision carried five interacting ones, and each
// extra strangled some legitimate transfer before it was tuned back, so the pair is kept
// deliberately plain. between them they bound the number of attempts; the only clock is on a
// reopen producing response headers, never on the transfer itself.
type resumableTransport struct {
	base        http.RoundTripper
	minSize     int64
	minProgress int64
	backoff     time.Duration
	maxStalls   int
	maxResumes  int
}

func newResumableTransport(base http.RoundTripper) *resumableTransport {
	if base == nil {
		base = http.DefaultTransport
	}

	return &resumableTransport{
		base:        base,
		minSize:     resumeMinSize,
		minProgress: minResumeProgress,
		backoff:     stallBackoff,
		maxStalls:   maxConsecutiveStalls,
		maxResumes:  maxTotalResumes,
	}
}

func (t *resumableTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || !t.shouldResume(req, resp) {
		return resp, err
	}

	resp.Body = &resumableBody{
		body:        resp.Body,
		req:         req,
		base:        t.base,
		total:       resp.ContentLength,
		minProgress: t.minProgress,
		backoff:     t.backoff,
		maxStalls:   t.maxStalls,
		maxResumes:  t.maxResumes,
		done:        make(chan struct{}),
	}

	return resp, nil
}

// shouldResume reports whether a response body is worth wrapping: a plain GET, not itself a range
// request so that the offset arithmetic starts from zero, which returned a body of known length
// large enough to be a layer blob.
//
// the length must be known because it is the only way to tell a stream that ended early from one
// that ended properly. it also keeps transparently decompressed responses out, since net/http sets
// ContentLength to -1 for those. the coding must be identity for the same reason: the offsets are
// always over wire bytes, and an identity segment cannot be spliced onto a stream that is not.
func (t *resumableTransport) shouldResume(req *http.Request, resp *http.Response) bool {
	return req.Method == http.MethodGet &&
		// Clone shares the body reader, so a reissued request would resend a consumed one
		(req.Body == nil || req.Body == http.NoBody) &&
		req.Header.Get("Range") == "" &&
		resp.Body != nil &&
		resp.StatusCode == http.StatusOK &&
		// the offsets are over wire bytes, so a stream that arrives in some coding cannot have an
		// identity segment spliced onto it. net/http only strips a coding it asked for itself, and
		// that sets ContentLength to -1, so an explicit one from the server survives to here
		identityEncoded(resp.Header.Get("Content-Encoding")) &&
		resp.ContentLength >= t.minSize
}

// resumableBody is the io.ReadCloser handed back in place of a large response body. it tracks how
// many bytes it has delivered so that a failed read can be continued with a Range request rather
// than restarting a transfer that may already be gigabytes along.
type resumableBody struct {
	req         *http.Request
	base        http.RoundTripper
	total       int64
	minProgress int64
	backoff     time.Duration
	maxStalls   int
	maxResumes  int

	// mu guards body, cancel and closed, which Close may touch from another goroutine while Read is
	// blocked or mid-resume. closing a Response.Body to abort a read is documented, legal usage of the
	// standard library, so the wrapper has to honour it.
	mu     sync.Mutex
	body   io.ReadCloser
	closed bool

	// cancel ends the reopen currently in flight, or the body it produced. without it Close cannot
	// interrupt a RoundTrip that has not yet returned, and a silent endpoint would hold the reader
	// for as long as the pull's context allows
	cancel context.CancelFunc

	// done is shut by Close, so a backoff in progress is abandoned rather than run to term
	done chan struct{}

	// the remainder belong to the reader and are not guarded: io.Reader admits a single reader
	offset   int64
	progress int64
	stalls   int
	resumes  int
	// failed latches the error that ended the stream, so a caller reading past a failure gets that
	// error back rather than starting the download over
	failed error
}

func (b *resumableBody) Read(p []byte) (int, error) {
	if b.failed != nil {
		return 0, b.failed
	}

	// never hand back more than the response declared, as an unwrapped net/http body would not
	remaining := b.total - b.offset
	if remaining <= 0 {
		return 0, b.latch(io.EOF)
	}

	if int64(len(p)) > remaining {
		p = p[:remaining]
	}

	for {
		//nolint:closecheck // borrowed from the struct, which owns it and closes it in Close
		body, err := b.currentBody()
		if err != nil {
			return 0, err
		}

		n, readErr := body.Read(p)
		b.record(n)

		if readErr == nil {
			return n, nil
		}
		if b.isClosed() {
			return n, b.latch(http.ErrBodyReadAfterClose)
		}
		if done, final := b.finished(); done {
			return n, b.latch(final)
		}
		if resumeErr := b.resume(shortRead(readErr)); resumeErr != nil {
			return n, b.latch(resumeErr)
		}
		if n > 0 {
			// hand back what this read managed; the next call continues on the new connection
			return n, nil
		}
	}
}

// shortRead renames the error of a stream that stopped before its declared length was delivered.
//
// net/http reports a body that simply ends as io.EOF, and the terminal failures below wrap their
// cause so that the sentinels this path raises stay classifiable. wrapping a bare io.EOF along with
// them would make errors.Is(err, io.EOF) true on a truncation, and a clean end of stream is exactly
// what a truncation must never be mistakable for: this repo's own file.IterateTar breaks on that
// test and returns nil, as do several go-containerregistry helpers, so a layer we failed to fetch
// would become a silently short SBOM -- the outcome this whole transport exists to prevent.
//
// it is called where the stream is known to be short, after finished() has ruled out a body that
// arrived in full.
func shortRead(readErr error) error {
	if errors.Is(readErr, io.EOF) {
		return io.ErrUnexpectedEOF
	}

	return readErr
}

// latch records the error that ended the stream so every subsequent Read returns it, rather than
// going round again on a body that is finished or dead.
func (b *resumableBody) latch(err error) error {
	b.failed = err

	return err
}

// record accounts for bytes just delivered, replenishing the stall budget once enough have arrived
// to count as real progress.
func (b *resumableBody) record(n int) {
	if n <= 0 {
		return
	}

	b.offset += int64(n)
	b.progress += int64(n)

	if b.progress >= b.minProgress {
		b.stalls = 0
		b.progress = 0
	}
}

// finished reports whether the read is over, and with which error. a body that arrived in full is
// finished whatever the connection made of it afterwards -- over HTTP/2 a RST_STREAM following the
// final DATA frame surfaces as a stream error rather than io.EOF -- and otherwise a cancelled
// context ends the read as its own error, so a truncated stream is never mistaken for a clean end.
func (b *resumableBody) finished() (bool, error) {
	if b.offset >= b.total {
		return true, io.EOF
	}

	if ctxErr := b.req.Context().Err(); ctxErr != nil {
		return true, ctxErr
	}

	return false, nil
}

// resume closes the failed connection and reopens the request from the current offset, retrying
// until it succeeds or runs out of budget.
func (b *resumableBody) resume(cause error) error {
	b.closeCurrent()

	for {
		b.resumes++
		if b.resumes > b.maxResumes {
			return fmt.Errorf("gave up resuming %s at byte %d after %d attempts in total: %w",
				forLog(b.req.URL), b.offset, b.maxResumes, cause)
		}

		b.stalls++
		if b.stalls > b.maxStalls {
			return fmt.Errorf("gave up resuming %s at byte %d after %d attempts without progress: %w",
				forLog(b.req.URL), b.offset, b.maxStalls, cause)
		}

		// Info rather than Debug: a run of these is the only sign an operator has that a slow pull
		// is recovering rather than wedged, and there are at most maxTotalResumes of them
		log.WithFields("url", forLog(b.req.URL), "offset", b.offset, "error", cause).
			Info("blob stream dropped, resuming from the current offset")

		// the first attempt after a drop goes out at once; only repeated failures back off. sleep
		// is called either way, so cancellation and Close are noticed on every attempt
		var delay time.Duration
		if b.stalls > 1 {
			delay = time.Duration(b.stalls-1) * b.backoff
		}

		if err := b.sleep(delay); err != nil {
			return err
		}

		//nolint:closecheck // ownership passes to adopt, which either stores it or closes it
		body, err := b.reopen()
		if err != nil {
			if permanentResumeError(err) {
				return fmt.Errorf("cannot resume %s at byte %d after %w: %w",
					forLog(b.req.URL), b.offset, cause, err)
			}

			// the same renaming the read error gets, and for the same reason: http.Transport hands
			// back a bare io.EOF when a peer accepts the connection, reads the request and closes
			// without answering -- which is what a drained CDN edge does, and is how this path is
			// reached at all
			cause = shortRead(err)

			continue
		}

		return b.adopt(body)
	}
}

// permanentResumeError reports whether retrying could possibly mend the error, so that a server
// which cannot resume fails at once rather than after the whole budget has been spent.
func permanentResumeError(err error) bool {
	return errors.Is(err, errNoRangeSupport) ||
		errors.Is(err, errObjectChanged) ||
		errors.Is(err, errCredentialsRejected) ||
		errors.Is(err, http.ErrBodyReadAfterClose)
}

// adopt installs a freshly opened body, unless the caller closed us while it was being opened.
func (b *resumableBody) adopt(body io.ReadCloser) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		closeQuietly(body)

		return http.ErrBodyReadAfterClose
	}

	b.body = body

	return nil
}

// sleep waits for d, or until the request's context is cancelled or the body closed, whichever
// comes first. the cancellation checks come before the zero-delay short-circuit so that a zero
// backoff, which is how the tests are configured, exercises the same behaviour as production.
func (b *resumableBody) sleep(d time.Duration) error {
	ctx := b.req.Context()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		return http.ErrBodyReadAfterClose
	default:
	}

	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		return http.ErrBodyReadAfterClose
	case <-timer.C:
		return nil
	}
}

// reopen issues the original request again for the bytes not yet delivered.
//
// the range is closed rather than open-ended. go-containerregistry's own registry -- which is what
// `crane registry serve` and ko run, and which a good many CI harnesses embed -- parses the header
// with Sscanf("bytes=%d-%d") and answers 416 to "bytes=N-", which would discard the layer on its
// first drop. every registry probed for this change answered a closed range correctly.
//
// identity is asked for because the stream so far is wire bytes: splicing a decoded segment onto it
// would be caught by a blob's digest but by nothing at all on a wrapped manifest.
func (b *resumableBody) reopen() (io.ReadCloser, error) {
	ctx, cancel := context.WithCancel(b.req.Context())
	if !b.adoptCancel(cancel) {
		cancel()

		return nil, http.ErrBodyReadAfterClose
	}

	req := b.req.Clone(ctx)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", b.offset, b.total-1))
	req.Header.Set("Accept-Encoding", "identity")

	// stopped once headers arrive, so the clock is on reaching the server rather than on the
	// transfer that follows. a timer that fires in the instant before Stop costs one attempt of the
	// budget, which is why this is not the only thing bounding the read
	headers := time.AfterFunc(reopenHeaderTimeout, cancel)
	resp, err := b.base.RoundTrip(req)
	timedOut := !headers.Stop()

	if err != nil {
		if timedOut {
			// the cancellation was ours, not the caller's. left bare it surfaces as "context
			// canceled", which is indistinguishable from a Ctrl-C nobody pressed
			return nil, fmt.Errorf("no response headers within %s: %w", reopenHeaderTimeout, err)
		}

		return nil, err
	}

	return b.acceptable(resp)
}

// adoptCancel stores the cancel belonging to the reopen now in flight, ending any it replaces, and
// reports whether the caller is still wanted -- Close may have arrived while it was being opened.
func (b *resumableBody) adoptCancel(cancel context.CancelFunc) bool {
	b.mu.Lock()
	previous := b.cancel
	b.cancel = cancel
	closed := b.closed
	b.mu.Unlock()

	if previous != nil {
		previous()
	}

	return !closed
}

// acceptable decides whether a reissued response may be spliced onto the stream, closing it if not.
// a 3xx is refused permanently rather than followed: this transport sits below
// go-containerregistry's checkRedirectSSRF, so honouring a Location from here would reach a host
// that client never vetted. refusedStatus sorts the rest into what is worth another attempt.
func (b *resumableBody) acceptable(resp *http.Response) (io.ReadCloser, error) {
	if resp.Body == nil {
		return nil, fmt.Errorf("%w: the server returned no body", errNoRangeSupport)
	}

	reject := func(err error) (io.ReadCloser, error) {
		closeQuietly(resp.Body)

		return nil, err
	}

	if resp.StatusCode != http.StatusPartialContent {
		return reject(b.refusedStatus(resp))
	}

	// the coding check guards a body about to be spliced, so it belongs below the status one. run
	// first, it read a busy server that serves its error page compressed as one that cannot resume
	// at all, and that verdict is permanent
	if !identityEncoded(resp.Header.Get("Content-Encoding")) {
		return reject(fmt.Errorf("%w: the resumed segment came back encoded", errNoRangeSupport))
	}

	start, end, total, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil {
		return reject(fmt.Errorf("%w: %w", errNoRangeSupport, err))
	}

	if start != b.offset {
		return reject(fmt.Errorf("%w: server resumed at byte %d, expected %d",
			errNoRangeSupport, start, b.offset))
	}

	// a server that ignored the Range and began the object again, while still echoing back the
	// range it was asked for, contradicts itself here: the body it declares is longer than the
	// span it claims to be sending. it cannot catch a server whose lengths agree and whose bytes
	// are simply wrong -- only the digest above can do that -- but it is the shape a server with
	// no real range support actually takes, and catching it here costs one request instead of a
	// whole layer
	if span := end - start + 1; end >= start && resp.ContentLength >= 0 && resp.ContentLength != span {
		return reject(fmt.Errorf("%w: Content-Range covers %d bytes but %d were sent",
			errNoRangeSupport, span, resp.ContentLength))
	}

	if total >= 0 && total != b.total {
		return reject(fmt.Errorf("%w: the blob is now %d bytes, it was %d when the read began",
			errObjectChanged, total, b.total))
	}

	return resp.Body, nil
}

// refusedStatus classifies a reopen that did not come back as 206.
//
// the distinction matters more than it looks. lumping every status into "this server cannot
// resume" made a busy CDN indistinguishable from one that has never heard of Range, and since that
// verdict is permanent a single 503 discarded however many gigabytes had already arrived -- while
// a dropped TCP connection from the very same host was survivable. that asymmetry was not intended.
func (b *resumableBody) refusedStatus(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		// an expired bearer token, or a pre-signed CDN URL that outlived the transfer
		return fmt.Errorf("%w: got %d resuming at byte %d", errCredentialsRejected,
			resp.StatusCode, b.offset)

	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError:
		// the server's own Retry-After is deliberately not read: the stall backoff already spaces
		// repeated attempts, and honouring a hint of minutes would only lengthen a pull that is
		// already failing
		return fmt.Errorf("%w: got %d resuming at byte %d", errResumeUnavailable,
			resp.StatusCode, b.offset)

	default:
		// 416, a plain 200, a redirect: the server understood the request and will not serve it
		return fmt.Errorf("%w: expected HTTP 206 resuming at byte %d, got %d",
			errNoRangeSupport, b.offset, resp.StatusCode)
	}
}

// identityEncoded reports whether a Content-Encoding may be spliced onto the stream so far.
//
// "identity" is not a valid content-coding for a response, but a server that is echoing the
// Accept-Encoding it was sent will return it anyway, and reopen always asks for identity. treating
// that as an encoding would permanently refuse to resume from such a server.
func identityEncoded(encoding string) bool {
	encoding = strings.TrimSpace(encoding)

	return encoding == "" || strings.EqualFold(encoding, "identity")
}

// currentBody returns the body to read from, or an error if the caller has closed us.
func (b *resumableBody) currentBody() (io.ReadCloser, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed || b.body == nil {
		return nil, http.ErrBodyReadAfterClose
	}

	return b.body, nil
}

func (b *resumableBody) isClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.closed
}

// closeCurrent drops the failed connection without marking the body closed to callers.
func (b *resumableBody) closeCurrent() {
	b.mu.Lock()
	body := b.body
	cancel := b.cancel
	b.body, b.cancel = nil, nil
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if body != nil {
		closeQuietly(body)
	}
}

func (b *resumableBody) Close() error {
	b.mu.Lock()
	first := !b.closed
	b.closed = true
	body := b.body
	cancel := b.cancel
	b.body, b.cancel = nil, nil
	b.mu.Unlock()

	if first {
		// wakes a backoff in progress; guarded by first so a second Close cannot panic
		close(b.done)
	}

	// ends a reopen that has not yet returned, which closing the body cannot reach
	if cancel != nil {
		cancel()
	}

	if body == nil {
		return nil
	}

	return body.Close()
}

// parseContentRange pulls the first and last byte positions and the total length out of a
// Content-Range header, e.g. the 4096, 8191 and 16384 in "bytes 4096-8191/16384". a total given as
// "*" is unknown and is reported as -1, as is a last byte position that will not parse.
func parseContentRange(header string) (int64, int64, int64, error) {
	malformed := func() (int64, int64, int64, error) {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q", header)
	}

	unit, spec, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(unit, "bytes") {
		return malformed()
	}

	span, size, ok := strings.Cut(strings.TrimSpace(spec), "/")
	if !ok {
		return malformed()
	}

	first, last, ok := strings.Cut(span, "-")
	if !ok {
		return malformed()
	}

	start, err := strconv.ParseInt(strings.TrimSpace(first), 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q: %w", header, err)
	}

	// the last byte position is advisory here: it is cross-checked against Content-Length when both
	// are present, but a server that omits or mangles it is not refused on that account alone
	end, err := strconv.ParseInt(strings.TrimSpace(last), 10, 64)
	if err != nil {
		end = -1
	}

	if size = strings.TrimSpace(size); size == "*" {
		return start, end, -1, nil
	}

	total, err := strconv.ParseInt(size, 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q: %w", header, err)
	}

	return start, end, total, nil
}

// forLog renders a URL without its query string, which for a CDN-signed blob URL carries the
// access token.
func forLog(u *url.URL) string {
	return u.Host + u.Path
}

func closeQuietly(c io.Closer) {
	if err := c.Close(); err != nil {
		log.WithFields("error", err).Trace("unable to close dropped blob stream")
	}
}
