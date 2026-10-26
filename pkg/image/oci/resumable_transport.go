package oci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anchore/stereoscope/internal/log"
)

const (
	// resumeMinSize is the blob size above which a body is worth wrapping. below it a drop costs
	// little to repeat, and a request that fails before its body starts flowing is already retried
	// by go-containerregistry.
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

	// statusClientClosedRequest is the non-standard 499 some proxies answer with when they abandon
	// an upstream request, which net/http has no constant for.
	statusClientClosedRequest = 499
)

// errResumeRefused marks a reopen that another attempt cannot mend: the server answered with a
// status that settles the matter, or with a segment that cannot be spliced onto the stream so far.
// anything else that goes wrong with a reopen spends a stall and is tried again.
var errResumeRefused = errors.New("the server refused to resume")

// resumableTransport wraps a RoundTripper so that a large GET whose body fails partway through is
// transparently resumed with a Range request from the byte already delivered.
//
// registries fronted by a CDN close the connection mid-blob on multi-gigabyte layers.
// go-containerregistry retries at the request level only, so once bytes are flowing a dropped
// connection surfaces as an unexpected EOF and the entire layer is discarded, however much of it
// had already arrived.
//
// only blobs are wrapped, and this sits below go-containerregistry's verification wrapper, so the
// digest and size of every reassembled stream are still checked end to end: a resume that splices
// in the wrong bytes fails the layer exactly as a corrupt single-shot download would. the offset,
// length and coding of each resumed response are checked here as well, so that a server which
// cannot resume costs one request rather than a whole layer before the digest says so.
//
// a reopen is not reissued from down here: it goes back to the registry URL through client, which
// authenticates and follows redirects, so it gets a fresh token and a freshly signed CDN URL rather
// than the ones that may have lapsed during a long transfer.
//
// termination is two budgets: consecutive attempts that deliver no meaningful progress, and
// reopens in total. between them they bound the number of attempts; the only clock is on a reopen
// producing response headers, never on the transfer itself.
type resumableTransport struct {
	base http.RoundTripper
	// client returns the client a reopen is issued through. it is called on each reopen so that
	// building it, which may ping the registry, waits until a resume is actually needed
	client        func(context.Context) (*http.Client, error)
	minSize       int64
	minProgress   int64
	backoff       time.Duration
	headerTimeout time.Duration
	maxStalls     int
	maxResumes    int
}

func newResumableTransport(base http.RoundTripper, client func(context.Context) (*http.Client, error)) *resumableTransport {
	return &resumableTransport{
		base:          base,
		client:        client,
		minSize:       resumeMinSize,
		minProgress:   minResumeProgress,
		backoff:       stallBackoff,
		headerTimeout: reopenHeaderTimeout,
		maxStalls:     maxConsecutiveStalls,
		maxResumes:    maxTotalResumes,
	}
}

func (t *resumableTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || !t.shouldResume(req, resp) {
		return resp, err
	}

	resp.Body = &resumableBody{
		t:     t,
		body:  resp.Body,
		req:   req,
		total: resp.ContentLength,
		done:  make(chan struct{}),
	}

	return resp, nil
}

// shouldResume reports whether a response body is worth wrapping: a plain GET for a registry blob,
// not itself a range request so that the offset arithmetic starts from zero, which returned a body
// of known length large enough to be worth resuming.
//
// only blobs are wrapped because only blobs are digest-verified above this transport. a manifest
// fetched by tag is not, so a resume spliced across a re-push of the tag would go undetected.
//
// the length must be known because it is the only way to tell a stream that ended early from one
// that ended properly. it also keeps transparently decompressed responses out, since net/http sets
// ContentLength to -1 for those. the coding must be identity for the same reason: the offsets are
// always over wire bytes, and an identity segment cannot be spliced onto a stream that is not.
func (t *resumableTransport) shouldResume(req *http.Request, resp *http.Response) bool {
	return req.Method == http.MethodGet &&
		isBlobRequest(req) &&
		// Clone shares the body reader, so a reissued request would resend a consumed one
		(req.Body == nil || req.Body == http.NoBody) &&
		req.Header.Get("Range") == "" &&
		resp.Body != nil &&
		resp.StatusCode == http.StatusOK &&
		// the offsets are over wire bytes, so a stream that arrives in some coding cannot have an
		// identity segment spliced onto it. net/http only strips a coding it asked for itself, and
		// that sets ContentLength to -1, so an explicit one from the server survives to here
		identityEncoded(resp.Header) &&
		resp.ContentLength >= t.minSize
}

// isBlobRequest reports whether a request is fetching a registry blob, a /v2/<name>/blobs/<digest>
// URL. a blob is usually served by a redirect to a CDN, so the request seen here is often the
// redirected one, whose path is the CDN's business; the client links each redirected request to
// the response that caused it, so the walk back to the first one finds the registry URL. the match
// is on the path alone, so a foreign layer at a URL of this shape (MCR's Windows layers are) is
// wrapped too, which is safe because go-containerregistry digest-verifies those the same way.
func isBlobRequest(req *http.Request) bool {
	// the last /blobs/, since a repository name may itself have a blobs component
	path := firstRequest(req).URL.Path
	i := strings.LastIndex(path, "/blobs/")
	if i < 0 {
		return false
	}
	digest := path[i+len("/blobs/"):]

	// an upload session lives under /blobs/uploads/, and a digest is algorithm:hex
	return strings.Contains(digest, ":") && !strings.Contains(digest, "/")
}

// firstRequest walks back through any redirects to the request the client began with.
func firstRequest(req *http.Request) *http.Request {
	for req.Response != nil && req.Response.Request != nil {
		req = req.Response.Request
	}

	return req
}

// resumableBody is the io.ReadCloser handed back in place of a large response body. it tracks how
// many bytes it has delivered so that a failed read can be continued with a Range request rather
// than restarting a transfer that may already be gigabytes along.
type resumableBody struct {
	t     *resumableTransport
	req   *http.Request
	total int64

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
		if resumeErr := b.resume(quoted(shortRead(readErr))); resumeErr != nil {
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
// would become a silently short SBOM, the outcome this whole transport exists to prevent.
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

	if b.progress >= b.t.minProgress {
		b.stalls = 0
		b.progress = 0
	}
}

// finished reports whether the read is over, and with which error. a body that arrived in full is
// finished whatever error came with its last bytes: net/http returns io.EOF alongside the final
// read of a Content-Length body, and a connection torn down at that moment reports some other
// error instead. otherwise a cancelled context ends the read as its own error, so a truncated
// stream is never mistaken for a clean end.
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

	// Info rather than Debug: a run of these is the only sign an operator has that a slow pull is
	// recovering rather than wedged
	log.WithFields("url", forLog(b.req), "offset", b.offset, "error", cause).
		Info("blob stream ended early, resuming from the current offset")

	for {
		b.resumes++
		if b.resumes > b.t.maxResumes {
			return fmt.Errorf("unable to resume %s at byte %d after %d attempts in total: %w",
				forLog(b.req), b.offset, b.t.maxResumes, cause)
		}

		b.stalls++
		if b.stalls > b.t.maxStalls {
			return fmt.Errorf("unable to resume %s at byte %d after %d attempts without progress: %w",
				forLog(b.req), b.offset, b.t.maxStalls, cause)
		}

		// the first attempt after a drop goes out at once; only repeated failures back off. sleep
		// is called either way, so cancellation and Close are noticed on every attempt
		var delay time.Duration
		if b.stalls > 1 {
			delay = time.Duration(b.stalls-1) * b.t.backoff
		}

		if err := b.sleep(delay); err != nil {
			return err
		}

		body, err := b.reopen()
		if err != nil {
			// a Close or a cancelled pull ends the reopen by design, so it ends the read too rather
			// than spending budget and logging a resume nobody is waiting for
			if b.isClosed() {
				return http.ErrBodyReadAfterClose
			}

			if ctxErr := b.req.Context().Err(); ctxErr != nil {
				return ctxErr
			}

			if errors.Is(err, errResumeRefused) {
				return fmt.Errorf("unable to resume %s at byte %d after %w: %w",
					forLog(b.req), b.offset, cause, err)
			}

			// the same renaming the read error gets, and for the same reason: http.Transport hands
			// back a bare io.EOF when a peer accepts the connection, reads the request and closes
			// without answering, which is what a drained CDN edge does, and is how this path is
			// reached at all
			cause = quoted(shortRead(err))
			log.WithFields("url", forLog(b.req), "offset", b.offset, "error", cause).
				Debug("unable to resume blob stream, retrying")

			continue
		}

		return b.adopt(body)
	}
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

// reopen issues the blob request again for the bytes not yet delivered.
//
// the range is closed rather than open-ended. go-containerregistry's own registry (which is what
// `crane registry serve` and ko run) parses the header with Sscanf("bytes=%d-%d") and answers 416 to
// "bytes=N-", which would discard the layer on its first drop.
//
// identity is asked for explicitly, overriding any Accept-Encoding cloned from the original request,
// because the stream so far is wire bytes and a segment in some other coding cannot be spliced onto
// it. net/http adds no coding of its own to a request carrying a Range.
func (b *resumableBody) reopen() (io.ReadCloser, error) {
	ctx, cancel := context.WithCancel(b.req.Context())
	if !b.adoptCancel(cancel) {
		cancel()

		return nil, http.ErrBodyReadAfterClose
	}

	// stopped once headers arrive, so the clock is on reaching the server rather than on the
	// transfer that follows
	headers := time.AfterFunc(b.t.headerTimeout, cancel)
	resp, err := b.roundTrip(ctx)

	if !headers.Stop() {
		// the cancellation was ours, not the caller's, so it is reported as a timeout rather than
		// wrapping the context error, which would read as a cancellation nobody asked for. a
		// response that raced the timer is already cancelled, so it is discarded
		if err == nil {
			closeQuietly(resp.Body)
		}

		return nil, fmt.Errorf("no response headers to the resumed range request within %s", b.t.headerTimeout)
	}

	// only an accepted body still needs its context; a failed attempt releases it at once rather
	// than leaving it to the next attempt or to Close
	if err != nil {
		cancel()

		return nil, err
	}

	body, err := b.acceptable(resp)
	if err != nil {
		cancel()
	}

	return body, err
}

// roundTrip sends the range request to the registry URL the blob was first requested from, rather
// than to the CDN hop that dropped, so the client can authenticate it and follow a fresh redirect.
func (b *resumableBody) roundTrip(ctx context.Context) (*http.Response, error) {
	client, err := b.t.client(ctx)
	if err != nil {
		return nil, err
	}

	req := firstRequest(b.req).Clone(ctx)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", b.offset, b.total-1))
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(req)

	// the client names the URL in its errors, and after a redirect that is a signed CDN URL
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}

	return resp, err
}

// checkRedirect is go-containerregistry's checkRedirectSSRF for the reopen client: a redirect may
// change host, but not to a loopback, private, link-local or unspecified address. a host made only of
// numbers is refused rather than decoded, since the resolver accepts legacy spellings such as 127.1
// that netip does not.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("%w: stopped after 10 redirects", errResumeRefused)
	}

	from, to := via[0].URL.Hostname(), req.URL.Hostname()
	if to == from {
		return nil
	}

	addr, err := netip.ParseAddr(to)
	if err != nil {
		if numericHost(to) {
			return fmt.Errorf("%w: redirect to the numeric host %q denied", errResumeRefused, to)
		}

		return nil
	}

	if addr = addr.Unmap(); addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsUnspecified() {
		return fmt.Errorf("%w: redirect from %q to private host %q denied", errResumeRefused, from, to)
	}

	return nil
}

// numericHost reports whether every dot-separated part of a host is a number in some base, as in
// 127.1, 0x7f000001 or 2130706433.
func numericHost(host string) bool {
	for _, part := range strings.Split(host, ".") {
		if _, err := strconv.ParseUint(part, 0, 32); err != nil {
			return false
		}
	}

	return true
}

// adoptCancel stores the cancel belonging to the reopen now in flight, ending any it replaces, and
// reports whether the caller is still wanted, since Close may have arrived while it was being opened.
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
func (b *resumableBody) acceptable(resp *http.Response) (io.ReadCloser, error) {
	reject := func(format string, args ...any) (io.ReadCloser, error) {
		closeQuietly(resp.Body)

		return nil, fmt.Errorf("%w: "+format, append([]any{errResumeRefused}, args...)...)
	}

	if resp.StatusCode != http.StatusPartialContent {
		closeQuietly(resp.Body)

		return nil, b.refusedStatus(resp)
	}

	// the coding check guards a body about to be spliced, so it comes after the status check: a
	// busy server may serve its error page compressed, and that must not read as a permanent refusal
	if !identityEncoded(resp.Header) {
		return reject("the resumed segment came back encoded")
	}

	start, end, total, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil {
		return reject("%w", err)
	}

	if start != b.offset {
		return reject("server resumed at byte %d, expected %d", start, b.offset)
	}

	if total >= 0 && total != b.total {
		return reject("the blob is now %d bytes, it was %d when the read began", total, b.total)
	}

	// a server that ignored the Range and began the object again, while still echoing back the
	// range it was asked for, declares a body longer than the span it claims to be sending
	if span := end - start + 1; resp.ContentLength >= 0 && resp.ContentLength != span {
		return reject("Content-Range covers %d bytes but %d were sent", span, resp.ContentLength)
	}

	if end >= b.total {
		return reject("Content-Range runs to byte %d, past the end at %d", end, b.total-1)
	}

	// a span that stops short of the end is legal, and some CDNs cap them. it is accepted, and when
	// it runs out Read resumes again, spending a resume just as a drop does

	// a body with no declared length is held to the span here instead, so bytes past its end are
	// never spliced in as if they continued it
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(resp.Body, end-start+1), resp.Body}, nil
}

// refusedStatus classifies a reopen that did not come back as 206. a busy server is worth another
// attempt, since a CDN edge unhealthy enough to sever a stream is the one liable to answer its
// reopen with a 503; a server that cannot or will not serve the range is not. a 401 or 403 is
// permanent too: the client has already re-authorized and followed a fresh redirect to get it.
func (b *resumableBody) refusedStatus(resp *http.Response) error {
	switch code := resp.StatusCode; {
	case code == http.StatusRequestTimeout || code == http.StatusTooManyRequests ||
		code == statusClientClosedRequest || code >= http.StatusInternalServerError:
		// every status go-containerregistry's own retry transport treats as transient, and the rest
		// of 5xx. Retry-After is not read: the stall backoff already spaces repeated attempts
		return fmt.Errorf("the server could not serve the range just now: got %d resuming at byte %d", code, b.offset)

	default:
		return fmt.Errorf("%w: expected HTTP 206 resuming at byte %d, got %d", errResumeRefused, b.offset, code)
	}
}

// identityEncoded reports whether a Content-Encoding may be spliced onto the stream so far.
//
// "identity" is not a valid content-coding for a response, but a server that is echoing the
// Accept-Encoding it was sent will return it anyway, and reopen always asks for identity. treating
// that as an encoding would permanently refuse to resume from such a server.
//
// every Content-Encoding line is checked, since a header repeated with a real coding after an
// identity one still means the body is encoded.
func identityEncoded(header http.Header) bool {
	for _, encoding := range header.Values("Content-Encoding") {
		if encoding = strings.TrimSpace(encoding); encoding != "" && !strings.EqualFold(encoding, "identity") {
			return false
		}
	}

	return true
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
	if body, _ := b.release(false); body != nil {
		closeQuietly(body)
	}
}

func (b *resumableBody) Close() error {
	body, first := b.release(true)
	if first {
		// wakes a backoff in progress; guarded by first so a second Close cannot panic
		close(b.done)
	}

	if body == nil {
		return nil
	}

	return body.Close()
}

// release detaches the current body and ends the reopen that produced it, or one still in flight,
// which closing the body cannot reach. closing also marks b closed, and reports whether this call was
// the one that did.
func (b *resumableBody) release(closing bool) (io.ReadCloser, bool) {
	b.mu.Lock()
	first := closing && !b.closed
	b.closed = b.closed || closing
	body, cancel := b.body, b.cancel
	b.body, b.cancel = nil, nil
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	return body, first
}

// parseContentRange pulls the first and last byte positions and the total length out of a
// Content-Range header, e.g. the 4096, 8191 and 16384 in "bytes 4096-8191/16384". a total given as
// "*" is unknown and is reported as -1. a range that runs backwards, or past a known total, is
// malformed.
func parseContentRange(header string) (int64, int64, int64, error) {
	malformed := func() (int64, int64, int64, error) {
		return 0, 0, 0, fmt.Errorf("malformed Content-Range %q", clip(header))
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
		return malformed()
	}

	end, err := strconv.ParseInt(strings.TrimSpace(last), 10, 64)
	if err != nil || start < 0 || end < start {
		return malformed()
	}

	if size = strings.TrimSpace(size); size == "*" {
		return start, end, -1, nil
	}

	total, err := strconv.ParseInt(size, 10, 64)
	if err != nil || total <= end {
		return malformed()
	}

	return start, end, total, nil
}

// forLog names a blob by the registry URL it was requested from, never the CDN URL it was redirected
// to: a signed CDN URL carries its access token in the query or, for some CDNs, in the path itself.
func forLog(req *http.Request) string {
	u := firstRequest(req).URL

	// escaped, since a foreign layer URL comes from the manifest and its path may decode to anything
	return u.Host + u.EscapedPath()
}

// maxErrorText caps how much of a server-influenced message reaches a log line or an error.
const maxErrorText = 200

// clip caps text a server chose before it goes into a log line or an error.
func clip(s string) string {
	if len(s) > maxErrorText {
		return s[:maxErrorText] + "..."
	}

	return s
}

// quotedError keeps an error's identity for errors.Is, but caps its text and escapes control
// characters. a failed reopen's error can carry bytes the server chose (some Go releases put a
// malformed header line into it raw), and it is logged and handed to the caller.
type quotedError struct{ err error }

func quoted(err error) error {
	if err == nil {
		return nil
	}

	return quotedError{err: err}
}

func (e quotedError) Error() string {
	q := strconv.Quote(clip(e.err.Error()))

	return q[1 : len(q)-1]
}

func (e quotedError) Unwrap() error { return e.err }

func closeQuietly(c io.Closer) {
	if err := c.Close(); err != nil {
		log.WithFields("error", err).Trace("unable to close dropped blob stream")
	}
}
