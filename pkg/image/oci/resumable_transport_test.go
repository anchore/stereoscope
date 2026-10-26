package oci

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flakyBlobServer serves a fixed payload, honouring Range requests, but aborts the connection
// partway through the body for the first dropsLeft attempts. this is how a CDN-fronted registry
// behaves on a multi-gigabyte layer.
type flakyBlobServer struct {
	mu           sync.Mutex
	payload      []byte
	dropsLeft    int
	bytesPerDrop int
	ranges       []string
	headers      []http.Header

	// rangeStatus, when non-zero, is served instead of 206 for a range request
	rangeStatus int

	// rangeStartSkew and totalSkew shift the start and total the server claims in Content-Range, as
	// a server resuming from the wrong place or serving a different object would
	rangeStartSkew int64
	totalSkew      int64

	// blankContentRange serves a 206 with an empty Content-Range, which the client cannot use
	blankContentRange bool

	// contentEncoding, when set, accompanies a range response
	contentEncoding string

	// abortAfterBody tears the connection down after the final byte rather than partway through
	abortAfterBody bool

	// restartOnRange echoes back the range that was asked for and then serves the whole object
	// from byte 0, as a server with no real range support does
	restartOnRange bool

	// transientStatus, when non-zero, is served for the first transientTimes range requests, after
	// which the server resumes correctly. transientTimes of 0 means for ever. this is a CDN edge
	// that is briefly unwell rather than one that cannot resume at all.
	transientStatus int
	transientTimes  int
	transientServed int

	// transientContentEncoding accompanies a transient refusal, as a CDN serving a pre-compressed
	// error page does whatever Accept-Encoding it was sent
	transientContentEncoding string
}

func (s *flakyBlobServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rangeHeader := r.Header.Get("Range")

	s.mu.Lock()
	s.ranges = append(s.ranges, rangeHeader)
	s.headers = append(s.headers, r.Header.Clone())
	drop := s.dropsLeft > 0
	if drop {
		s.dropsLeft--
	}

	transient := false
	if rangeHeader != "" && s.transientStatus != 0 &&
		(s.transientTimes == 0 || s.transientServed < s.transientTimes) {
		transient = true
		s.transientServed++
	}
	s.mu.Unlock()

	if transient {
		if s.transientContentEncoding != "" {
			w.Header().Set("Content-Encoding", s.transientContentEncoding)
		}
		w.WriteHeader(s.transientStatus)

		return
	}

	start, end := int64(0), int64(-1)
	status := http.StatusOK
	remaining := s.payload

	if rangeHeader != "" {
		start, end = parseRangeRequest(rangeHeader)
		status = http.StatusPartialContent
		if s.rangeStatus != 0 {
			status = s.rangeStatus
		}

		remaining = limitToRange(s.payload[start:], start, end)

		// what Content-Range will claim; normally the same as what is sent
		declared := int64(len(remaining))

		if s.restartOnRange {
			remaining = s.payload
		}

		if s.contentEncoding != "" {
			w.Header().Set("Content-Encoding", s.contentEncoding)
		}

		if s.blankContentRange {
			w.Header().Set("Content-Range", "")
		} else {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d",
				start+s.rangeStartSkew, start+declared-1, int64(len(s.payload))+s.totalSkew))
		}
	}

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.Itoa(len(remaining)))
	w.WriteHeader(status)

	if drop {
		_, _ = w.Write(remaining[:s.bytesPerDrop])
		// force the partial write onto the wire before tearing the connection down, so the client
		// genuinely sees a truncated body rather than nothing at all
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}

	_, _ = w.Write(remaining)

	if s.abortAfterBody {
		// every declared byte has been delivered; only then is the connection torn down
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}
}

func (s *flakyBlobServer) seenRanges() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ranges...)
}

func (s *flakyBlobServer) seenHeaders() []http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]http.Header(nil), s.headers...)
}

// trickleServer delivers a single byte and then aborts, for ever. every attempt technically makes
// progress, which is what a budget replenished by any progress at all cannot survive.
type trickleServer struct {
	mu       sync.Mutex
	payload  []byte
	requests int
}

func (s *trickleServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests++
	s.mu.Unlock()

	start := int64(0)
	status := http.StatusOK
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		start, _ = parseRangeRequest(rangeHeader)
		status = http.StatusPartialContent
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(s.payload)-1, len(s.payload)))
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(s.payload)-int(start)))
	w.WriteHeader(status)

	_, _ = w.Write(s.payload[start : start+1])
	w.(http.Flusher).Flush()
	panic(http.ErrAbortHandler)
}

func (s *trickleServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}

// parseRangeRequest returns the inclusive bounds of a closed range, or an end of -1 for an
// open-ended one. the fixtures honour the end so that they model what the transport actually sends.
func parseRangeRequest(header string) (int64, int64) {
	spec := strings.TrimPrefix(header, "bytes=")

	first, last, _ := strings.Cut(spec, "-")

	start, err := strconv.ParseInt(first, 10, 64)
	if err != nil {
		panic(err)
	}

	if last == "" {
		return start, -1
	}

	end, err := strconv.ParseInt(last, 10, 64)
	if err != nil {
		panic(err)
	}

	return start, end
}

// limitToRange trims a payload tail to the inclusive end of a requested range.
func limitToRange(tail []byte, start, end int64) []byte {
	if end < 0 {
		return tail
	}

	if want := end - start + 1; want >= 0 && want < int64(len(tail)) {
		return tail[:want]
	}

	return tail
}

func payloadOfSize(n int) []byte {
	payload := make([]byte, n)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	return payload
}

// blobPath is where the fixtures are fetched from, since only registry blobs are resumed.
const blobPath = "/v2/img/blobs/sha256:abc"

// newTestTransport builds a transport with no backoff, so the tests need no real delays.
func newTestTransport() *resumableTransport {
	transport := newResumableTransport(http.DefaultTransport, directClient(http.DefaultTransport))
	transport.backoff = 0
	return transport
}

// directClient reopens through base alone, following redirects as the registry client does but
// without authenticating.
func directClient(base http.RoundTripper) reopenClientFunc {
	client := &http.Client{Transport: base, CheckRedirect: checkRedirect}

	return func(context.Context) (*http.Client, error) { return client, nil }
}

func newTestClient() *http.Client {
	return &http.Client{Transport: newTestTransport()}
}

func TestResumableTransport_reassemblesBlobAcrossDrops(t *testing.T) {
	payload := payloadOfSize(8192)
	server := &flakyBlobServer{payload: payload, dropsLeft: 3, bytesPerDrop: 1000}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, got, "the reassembled body should be byte-identical to the original")

	body := resp.Body.(*resumableBody)
	assert.Nil(t, body.body, "the last segment should be released at EOF, not held for Close")
	assert.Nil(t, body.cancel, "the last resume's context should be released at EOF, not held for Close")

	// the first attempt carries no Range; each resume asks for exactly the bytes still owed, as a
	// closed range, since an open-ended one is answered with 416 by go-containerregistry's own registry
	assert.Equal(t, []string{"", "bytes=1000-8191", "bytes=2000-8191", "bytes=3000-8191"},
		server.seenRanges())
}

func TestResumableTransport_survivesManyDropsWhileMakingProgress(t *testing.T) {
	payload := payloadOfSize(2 * 1024 * 1024)

	// far more drops than the stall budget, each delivering well over minResumeProgress, so real
	// progress must keep the budget topped up or a long but healthy transfer would be abandoned
	server := &flakyBlobServer{payload: payload, dropsLeft: 14, bytesPerDrop: 128 * 1024}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	assert.Greater(t, len(server.seenRanges()), maxConsecutiveStalls)
}

func TestResumableTransport_terminatesAgainstATricklingServer(t *testing.T) {
	// one byte per connection is progress by any naive measure, so a budget replenished by any
	// n > 0 would never be spent and the read would hang indefinitely
	server := &trickleServer{payload: payloadOfSize(1 << 20)}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	done := make(chan error, 1)
	go func() {
		_, readErr := io.ReadAll(resp.Body)
		done <- readErr
	}()

	select {
	case readErr := <-done:
		require.Error(t, readErr)
		assert.Contains(t, readErr.Error(), "without progress")
		assert.LessOrEqual(t, server.count(), maxConsecutiveStalls+1)
	case <-time.After(10 * time.Second):
		t.Fatalf("read never terminated; %d requests issued", server.count())
	}
}

func TestResumableTransport_givesUpWhenNoProgressIsMade(t *testing.T) {
	// every attempt delivers zero bytes, so the stall budget is never replenished
	server := &flakyBlobServer{payload: payloadOfSize(8192), dropsLeft: 1000, bytesPerDrop: 0}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without progress")

	// the initial attempt plus exactly the stall budget, and no more
	assert.Len(t, server.seenRanges(), maxConsecutiveStalls+1)
}

func TestResumableTransport_refusesAResumeThatIsNotPartialContent(t *testing.T) {
	// a server answering a range request with the whole body would restart the stream, duplicating
	// everything already written
	server := &flakyBlobServer{
		payload:      payloadOfSize(8192),
		dropsLeft:    1000,
		bytesPerDrop: 512,
		rangeStatus:  http.StatusOK,
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	assert.ErrorIs(t, err, errResumeRefused)
	assert.Contains(t, err.Error(), "after ", "the original drop should survive in the error chain")

	// a server that will not resume is refused at once rather than after the whole budget, so the
	// failure is no slower than it was before resuming existed
	assert.Len(t, server.seenRanges(), 2)
}

func TestResumableTransport_refusesAResumeFromTheWrongOffset(t *testing.T) {
	server := &flakyBlobServer{
		payload:        payloadOfSize(8192),
		dropsLeft:      1000,
		bytesPerDrop:   512,
		rangeStartSkew: 16,
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	assert.ErrorIs(t, err, errResumeRefused)
	assert.Contains(t, err.Error(), "server resumed at byte")
	assert.Len(t, server.seenRanges(), 2)
}

func TestResumableTransport_refusesAResumeWithAnUnusableContentRange(t *testing.T) {
	// a server that cannot form a Content-Range will not form one on the fifth attempt either
	server := &flakyBlobServer{
		payload:           payloadOfSize(8192),
		dropsLeft:         1000,
		bytesPerDrop:      512,
		blankContentRange: true,
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	assert.ErrorIs(t, err, errResumeRefused)
	assert.Len(t, server.seenRanges(), 2)
}

func TestResumableTransport_refusesAResumeWhenTheBlobLengthChanged(t *testing.T) {
	// a Content-Range reporting a different total is direct evidence the object is not the one the
	// read began with, and costs nothing to check
	server := &flakyBlobServer{
		payload:      payloadOfSize(8192),
		dropsLeft:    1,
		bytesPerDrop: 1000,
		totalSkew:    64,
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	assert.ErrorIs(t, err, errResumeRefused)
	assert.Contains(t, err.Error(), "the blob is now")
	assert.Len(t, server.seenRanges(), 2)
}

func TestResumableTransport_refusesAnEncodedSegment(t *testing.T) {
	// splicing decoded bytes onto a stream of wire bytes would only be caught by the digest, once the
	// whole layer had been read
	server := &flakyBlobServer{
		payload:         payloadOfSize(8192),
		dropsLeft:       1,
		bytesPerDrop:    1000,
		contentEncoding: "gzip",
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	assert.ErrorIs(t, err, errResumeRefused)
	assert.Contains(t, err.Error(), "came back encoded")
}

func TestResumableTransport_asksForIdentityOnResume(t *testing.T) {
	// the reissued request is a clone, so an Accept-Encoding set on the original would ride along;
	// the resume pins identity so the segment arrives in the same coding as the stream so far
	server := &flakyBlobServer{payload: payloadOfSize(8192), dropsLeft: 1, bytesPerDrop: 1000}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	headers := server.seenHeaders()
	require.Len(t, headers, 2)
	assert.Equal(t, "identity", headers[1].Get("Accept-Encoding"), "the resume must pin the coding")
}

func TestResumableTransport_neverReplaysCredentialsOnAResume(t *testing.T) {
	// go-containerregistry sets Authorization on the request in place. a resume that replayed it
	// would hand it to every redirect the reopen client follows, to hosts the pull never sent it to,
	// so only the reopen client may add credentials. the user agent is kept
	server := &flakyBlobServer{payload: payloadOfSize(8192), dropsLeft: 2, bytesPerDrop: 1000}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+blobPath, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer sekrit")
	req.Header.Set("User-Agent", "stereoscope-test")

	resp, err := newTestClient().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	headers := server.seenHeaders()
	require.Len(t, headers, 3)
	for i, h := range headers[1:] {
		assert.Empty(t, h.Get("Authorization"), "resume %d replayed the original credentials", i)
		assert.Equal(t, "stereoscope-test", h.Get("User-Agent"), "resume %d lost its user agent", i)
	}
}

func TestResumableTransport_resumesThroughARedirectToACDN(t *testing.T) {
	// mcr.microsoft.com answers the blob request with a 307 to a CDN, and it is the CDN hop that
	// drops. a resume goes back to the registry for a fresh redirect, and the range rides along
	payload := payloadOfSize(8192)
	cdn := &flakyBlobServer{payload: payload, dropsLeft: 2, bytesPerDrop: 1000}

	cdnSrv := httptest.NewServer(cdn)
	t.Cleanup(cdnSrv.Close)

	var mu sync.Mutex
	var registryRanges []string
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		registryRanges = append(registryRanges, r.Header.Get("Range"))
		mu.Unlock()
		http.Redirect(w, r, cdnSrv.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(registry.Close)

	resp, err := newTestClient().Get(registry.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	assert.Equal(t, []string{"", "bytes=1000-8191", "bytes=2000-8191"}, cdn.seenRanges())
	assert.Equal(t, []string{"", "bytes=1000-8191", "bytes=2000-8191"}, registryRanges,
		"each resume should be redirected afresh by the registry")
}

func TestResumableTransport_resumesWithAFreshTokenAndSignedURL(t *testing.T) {
	// the pull outlives both its bearer token and the signed CDN URL it was redirected to. a resume
	// reissued from the CDN hop with the original headers would earn a 401 or 403; one sent through
	// the reopen client is re-authorized and redirected to a freshly signed URL
	payload := payloadOfSize(8192)

	var mu sync.Mutex
	signature := 1
	token := "Bearer first"
	spent := map[string]bool{}

	cdn := &flakyBlobServer{payload: payload, dropsLeft: 1, bytesPerDrop: 1000}
	cdnSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sig := r.URL.Query().Get("sig")
		valid := sig == strconv.Itoa(signature) && !spent[sig]
		spent[sig] = true
		mu.Unlock()
		if !valid {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		cdn.ServeHTTP(w, r)
	}))
	t.Cleanup(cdnSrv.Close)

	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// both lapse as soon as they have been used, so only a re-authorized, re-redirected request
		// can succeed
		signature++
		token = "Bearer second"
		http.Redirect(w, r, fmt.Sprintf("%s?sig=%d", cdnSrv.URL, signature), http.StatusTemporaryRedirect)
	}))
	t.Cleanup(registry.Close)

	// stands in for go-containerregistry's bearer transport, which sets the token on registry requests
	authorize := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == strings.TrimPrefix(registry.URL, "http://") {
			req.Header.Set("Authorization", "Bearer second")
		}
		return http.DefaultTransport.RoundTrip(req)
	})
	reopen := &http.Client{Transport: authorize, CheckRedirect: checkRedirect}

	transport := newResumableTransport(http.DefaultTransport, func(context.Context) (*http.Client, error) { return reopen, nil })
	transport.backoff = 0

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, registry.URL+blobPath, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer first")

	resp, err := (&http.Client{Transport: transport}).Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	assert.Equal(t, []string{"", "bytes=1000-8191"}, cdn.seenRanges())
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func Test_checkRedirect(t *testing.T) {
	via := []*http.Request{httptest.NewRequest(http.MethodGet, "https://registry.example.com/v2/img/blobs/sha256:abc", nil)}

	tests := []struct {
		to      string
		allowed bool
	}{
		{to: "https://registry.example.com/elsewhere", allowed: true},
		{to: "https://cdn.example.com/blob?sig=1", allowed: true},
		{to: "https://203.0.113.7/blob", allowed: true},
		{to: "https://127.0.0.1/blob"},
		{to: "https://10.0.0.1/blob"},
		{to: "https://169.254.169.254/latest/meta-data"},
		{to: "https://[::1]/blob"},
		{to: "https://[::ffff:127.0.0.1]/blob"},
		{to: "https://0.0.0.0/blob"},
		// legacy spellings the resolver accepts and netip does not
		{to: "https://127.1/blob"},
		{to: "https://0x7f000001/blob"},
		{to: "https://2130706433/blob"},
		// full-width digits, which the transport maps onto 127.0.0.1 when it dials
		{to: "https://１２７．０．０．１/blob"},
		{to: "https://cdn.exämple.com/blob"},
		// a downgrade, even to the same host
		{to: "http://registry.example.com/blob"},
		{to: "http://cdn.example.com/blob"},
	}

	for _, tt := range tests {
		t.Run(tt.to, func(t *testing.T) {
			err := checkRedirect(httptest.NewRequest(http.MethodGet, tt.to, nil), via)
			if tt.allowed {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, errResumeRefused)
		})
	}

	assert.ErrorIs(t, checkRedirect(via[0], make([]*http.Request, 10)), errResumeRefused, "redirects are capped")

	long := httptest.NewRequest(http.MethodGet, "https://"+strings.Repeat("a", 1<<16)+".example/", nil)
	err := checkRedirect(long, []*http.Request{httptest.NewRequest(http.MethodGet, "https://10.0.0.1/", nil)})
	require.NoError(t, err, "a public name is allowed whatever its length")
	err = checkRedirect(httptest.NewRequest(http.MethodGet, "https://"+strings.Repeat("１", 1<<16)+"/", nil), via)
	require.Error(t, err)
	assert.Less(t, len(err.Error()), maxErrorText+128, "the refused host must be clipped in the error")
}

func TestResumableTransport_neverReturnsMoreThanWasDeclared(t *testing.T) {
	// a 206 whose Content-Range checks out but whose body runs past the end of the range must not be
	// spliced in whole: an unwrapped net/http body caps at Content-Length, and so must this one.
	//
	// the segment is served chunked, so there is no Content-Length to cross-check against the
	// Content-Range, and holding it to the span is the only thing withholding the surplus. a segment
	// that does declare a length inconsistent with its range is refused outright instead, which
	// TestResumableTransport_refusesASegmentLongerThanTheRangeItClaims covers
	payload := payloadOfSize(8192)
	overrun := payloadOfSize(500)

	var mu sync.Mutex
	served := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		served++
		first := served == 1
		mu.Unlock()

		if first {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload[:1000])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}

		start, _ := parseRangeRequest(r.Header.Get("Range"))
		tail := append(append([]byte{}, payload[start:]...), overrun...)

		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(tail)
	}))
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Len(t, got, len(payload), "the surplus past Content-Length must be withheld")
	assert.Equal(t, payload, got)
}

func TestResumableTransport_resumesOverHTTP2(t *testing.T) {
	// most registries serve HTTP/2, where a truncated body surfaces through a different path in
	// net/http than it does over HTTP/1.1
	payload := payloadOfSize(2 * 1024 * 1024)
	server := &flakyBlobServer{payload: payload, dropsLeft: 3, bytesPerDrop: 200 * 1024}

	srv := httptest.NewUnstartedServer(server)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)

	transport := newResumableTransport(srv.Client().Transport, directClient(srv.Client().Transport))
	transport.backoff = 0

	resp, err := (&http.Client{Transport: transport}).Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	require.Equal(t, "HTTP/2.0", resp.Proto, "the fixture must actually be serving HTTP/2")
	require.IsType(t, &resumableBody{}, resp.Body)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	assert.Len(t, server.seenRanges(), 4)
}

func TestResumableTransport_completesWhenTheConnectionIsResetAfterTheLastByte(t *testing.T) {
	// the declared length is what says a body is complete, so tearing the connection down once the
	// last byte is out must not cost a resume, which would ask for a range past the end
	payload := payloadOfSize(2 * 1024 * 1024)
	server := &flakyBlobServer{payload: payload, abortAfterBody: true}

	srv := httptest.NewUnstartedServer(server)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)

	transport := newResumableTransport(srv.Client().Transport, directClient(srv.Client().Transport))
	transport.backoff = 0

	resp, err := (&http.Client{Transport: transport}).Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	assert.Len(t, server.seenRanges(), 1, "a complete body must not be resumed at all")
}

// errAfterReader hands back its bytes and the given error together on the read that drains it.
type errAfterReader struct {
	data []byte
	err  error
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.err
	}
	return n, nil
}

func (r *errAfterReader) Close() error { return nil }

func TestResumableTransport_aBodyDeliveredInFullIsCompleteWhateverErrorCameWithIt(t *testing.T) {
	// the read that delivers the last bytes may carry io.EOF or a connection error. either way every
	// declared byte has arrived, and resuming would ask for a range past the end
	payload := payloadOfSize(4096)

	for _, readErr := range []error{io.EOF, errors.New("connection reset by peer")} {
		t.Run(readErr.Error(), func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com/v2/img/blobs/sha256:abc", nil)
			body := &resumableBody{
				t:     newTestTransport(),
				body:  &errAfterReader{data: payload, err: readErr},
				req:   req,
				total: int64(len(payload)),
				done:  make(chan struct{}),
			}

			got, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.Equal(t, payload, got)
		})
	}
}

func TestResumableTransport_readAfterCloseDoesNotRestartTheDownload(t *testing.T) {
	server := &flakyBlobServer{payload: payloadOfSize(8192)}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)

	require.IsType(t, &resumableBody{}, resp.Body, "the fixture must produce a wrapped body")

	buf := make([]byte, 16)
	_, err = resp.Body.Read(buf)
	require.NoError(t, err)

	before := len(server.seenRanges())
	require.NoError(t, resp.Body.Close())

	n, err := resp.Body.Read(buf)
	assert.Zero(t, n)
	require.Error(t, err, "a closed body must not go on serving bytes")
	assert.Len(t, server.seenRanges(), before, "a read after close must not open a new connection")
}

func TestResumableTransport_closeAbandonsABackoffInProgress(t *testing.T) {
	// zero bytes per attempt means the stall count climbs and the backoff genuinely engages
	server := &flakyBlobServer{payload: payloadOfSize(1 << 20), dropsLeft: 10_000, bytesPerDrop: 0}

	// the first reopen goes out immediately and the next waits a full backoff, so once the server
	// has seen the first reopen the reader is on its way into that backoff
	reopened := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			once.Do(func() { close(reopened) })
		}
		server.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	transport := newTestTransport()
	transport.backoff = 10 * time.Second
	transport.maxStalls = 1_000_000

	resp, err := (&http.Client{Transport: transport}).Get(srv.URL + blobPath)
	require.NoError(t, err)

	returned := make(chan struct{})
	go func() {
		_, _ = io.ReadAll(resp.Body)
		close(returned)
	}()

	<-reopened
	closedAt := time.Now()
	require.NoError(t, resp.Body.Close())

	select {
	case <-returned:
		assert.Less(t, time.Since(closedAt), 2*time.Second,
			"Close must abandon the backoff, which here would otherwise run for 10s")
	case <-time.After(5 * time.Second):
		t.Fatal("the reader never returned after Close")
	}

	// reopens are synchronous, so once the reader has returned nothing more can be in flight
	assert.Len(t, server.seenRanges(), 2, "a closed body must not issue a further request on its way out")
}

func TestResumableTransport_doesNotResumeACancelledRequest(t *testing.T) {
	server := &flakyBlobServer{payload: payloadOfSize(8192), dropsLeft: 1000, bytesPerDrop: 512}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+blobPath, nil)
	require.NoError(t, err)

	// no budget at all, so a cancellation mistaken for a drop would report giving up instead
	transport := newTestTransport()
	transport.maxResumes = 0

	resp, err := (&http.Client{Transport: transport}).Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	cancel()

	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled, "cancellation should surface as such, not as a clean EOF")
	assert.NotContains(t, err.Error(), "attempts", "a cancellation is not a dropped stream to resume")

	before := len(server.seenRanges())
	_, again := resp.Body.Read(make([]byte, 16))
	assert.ErrorIs(t, again, context.Canceled, "the terminal error should be latched")
	assert.Len(t, server.seenRanges(), before, "reading past a terminal error must not reopen anything")
}

func TestResumableTransport_leavesUnknownLengthResponsesAlone(t *testing.T) {
	// without a Content-Length there is no way to tell a truncated stream from a complete one, so
	// such a body is passed through untouched rather than resumed on a guess
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// flushing before the first write forces chunked encoding, so no Content-Length is sent
		w.(http.Flusher).Flush()
		_, _ = w.Write(payloadOfSize(2048))
	}))
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	require.Equal(t, int64(-1), resp.ContentLength, "the fixture must not send a Content-Length")

	_, ok := resp.Body.(*resumableBody)
	assert.False(t, ok, "a response of unknown length should not be wrapped")
}

func TestResumableTransport_shouldResume(t *testing.T) {
	get := func(rangeHeader string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/v2/img/blobs/sha256:abc", nil)
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
		return req
	}

	response := func(status int, length int64) *http.Response {
		return &http.Response{
			StatusCode:    status,
			ContentLength: length,
			Body:          io.NopCloser(strings.NewReader("")),
			Header:        http.Header{},
		}
	}

	// redirected builds the request the client sends after following a redirect, which links back
	// to the request that drew it
	redirected := func(from, to string) *http.Request {
		original := httptest.NewRequest(http.MethodGet, from, nil)
		req := httptest.NewRequest(http.MethodGet, to, nil)
		req.Response = &http.Response{StatusCode: http.StatusTemporaryRedirect, Request: original}
		return req
	}

	encoded := func(coding string) *http.Response {
		resp := response(http.StatusOK, 1024)
		resp.Header.Set("Content-Encoding", coding)
		return resp
	}

	transport := newResumableTransport(http.DefaultTransport, nil)

	tests := []struct {
		name     string
		req      *http.Request
		resp     *http.Response
		expected bool
	}{
		{
			name:     "a large plain GET is wrapped",
			req:      get(""),
			resp:     response(http.StatusOK, 1024),
			expected: true,
		},
		{
			// the offsets are wire bytes, so an identity segment cannot be spliced onto a stream
			// that arrived in some coding. net/http leaves an explicit one from the server in place
			name:     "an encoded response is left alone",
			req:      get(""),
			resp:     encoded("br"),
			expected: false,
		},
		{
			name:     "an identity coding is treated as no coding at all",
			req:      get(""),
			resp:     encoded("identity"),
			expected: true,
		},
		{
			name:     "an empty response is left alone",
			req:      get(""),
			resp:     response(http.StatusOK, 0),
			expected: false,
		},
		{
			name:     "an unknown content length is left alone",
			req:      get(""),
			resp:     response(http.StatusOK, -1),
			expected: false,
		},
		{
			// a server that ignores Range answers 200, and the offsets would then be wrong
			name:     "a caller's own range request is left alone",
			req:      get("bytes=10-"),
			resp:     response(http.StatusOK, 1024),
			expected: false,
		},
		{
			name:     "a non-200 response is left alone",
			req:      get(""),
			resp:     response(http.StatusNotFound, 1024),
			expected: false,
		},
		{
			name:     "a HEAD is left alone",
			req:      httptest.NewRequest(http.MethodHead, "http://example.com/v2/img/blobs/sha256:abc", nil),
			resp:     response(http.StatusOK, 1024),
			expected: false,
		},
		{
			name:     "a response with no body is left alone",
			req:      get(""),
			resp:     &http.Response{StatusCode: http.StatusOK, ContentLength: 1024},
			expected: false,
		},
		{
			name:     "a manifest is left alone, since nothing verifies one fetched by tag",
			req:      httptest.NewRequest(http.MethodGet, "http://example.com/v2/img/manifests/latest", nil),
			resp:     response(http.StatusOK, 1024),
			expected: false,
		},
		{
			name:     "a blob in a repository with a blobs component is wrapped",
			req:      httptest.NewRequest(http.MethodGet, "http://example.com/v2/org/blobs/app/blobs/sha256:abc", nil),
			resp:     response(http.StatusOK, 1024),
			expected: true,
		},
		{
			name:     "a manifest in a repository with a blobs component is left alone",
			req:      httptest.NewRequest(http.MethodGet, "http://example.com/v2/org/blobs/app/manifests/sha256:abc", nil),
			resp:     response(http.StatusOK, 1024),
			expected: false,
		},
		{
			name:     "an upload session is left alone",
			req:      httptest.NewRequest(http.MethodGet, "http://example.com/v2/img/blobs/uploads/1234", nil),
			resp:     response(http.StatusOK, 1024),
			expected: false,
		},
		{
			name:     "a CDN hop is wrapped when the registry request it came from was for a blob",
			req:      redirected("http://example.com/v2/img/blobs/sha256:abc", "http://cdn.example.com/x/y?sig=1"),
			resp:     response(http.StatusOK, 1024),
			expected: true,
		},
		{
			name:     "a CDN hop is left alone when the registry request it came from was not for a blob",
			req:      redirected("http://example.com/v2/img/manifests/latest", "http://cdn.example.com/x/y?sig=1"),
			resp:     response(http.StatusOK, 1024),
			expected: false,
		},
		{
			name:     "a request carrying a body is left alone",
			req:      httptest.NewRequest(http.MethodGet, "http://example.com/v2/img/blobs/sha256:abc", strings.NewReader("x")),
			resp:     response(http.StatusOK, 1024),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, transport.shouldResume(tt.req, tt.resp))
		})
	}
}

func TestParseContentRange(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		start   int64
		end     int64
		total   int64
		wantErr bool
	}{
		{name: "a well formed range", header: "bytes 4096-8191/16384", start: 4096, end: 8191, total: 16384},
		{name: "a range starting at zero", header: "bytes 0-15/16", start: 0, end: 15, total: 16},
		{name: "an unknown total", header: "bytes 512-1023/*", start: 512, end: 1023, total: -1},
		{name: "extra whitespace", header: "  bytes  4096-8191/16384 ", start: 4096, end: 8191, total: 16384},
		{name: "a missing last byte", header: "bytes 4096-/16384", wantErr: true},
		{name: "a negative last byte", header: "bytes 5--3/20", wantErr: true},
		{name: "a range that runs backwards", header: "bytes 9-5/20", wantErr: true},
		{name: "a negative total", header: "bytes 5-9/-1", wantErr: true},
		{name: "a range past the total", header: "bytes 5-20/20", wantErr: true},
		{name: "an empty header", header: "", wantErr: true},
		{name: "a missing unit", header: "4096-8191/16384", wantErr: true},
		{name: "a unit that is not bytes", header: "items 0-5/10", wantErr: true},
		{name: "a missing total", header: "bytes 4096-8191", wantErr: true},
		{name: "a non-numeric start", header: "bytes abc-8191/16384", wantErr: true},
		{name: "a non-numeric total", header: "bytes 4096-8191/many", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, total, err := parseContentRange(tt.header)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.start, start)
			assert.Equal(t, tt.end, end)
			assert.Equal(t, tt.total, total)
		})
	}
}

func TestParseContentRange_echoesOnlyAPrefixOfAMalformedHeader(t *testing.T) {
	// the server picks the header, and the error ends up in a caller's output
	for _, header := range []string{
		"bytes " + strings.Repeat("9", 1<<20) + "-1/2",
		"bytes 0-1/" + strings.Repeat("x", 1<<20),
	} {
		_, _, _, err := parseContentRange(header)
		require.Error(t, err)
		assert.Less(t, len(err.Error()), maxErrorText+64)
	}
}

func Test_quoted(t *testing.T) {
	// a reopen's error can carry raw bytes from a hostile server, and it reaches logs and terminals
	inner := fmt.Errorf("malformed MIME header line: X: \x1b]0;pwned\x07%s: %w", strings.Repeat("y", 1<<20), io.ErrUnexpectedEOF)
	err := quoted(inner)

	assert.NotContains(t, err.Error(), "\x1b", "control characters must be escaped")
	assert.Contains(t, err.Error(), `\x1b]0;pwned\a`)
	assert.Less(t, len(err.Error()), maxErrorText+64, "the text must be capped")
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF, "the error must stay classifiable")
}

func TestResumableTransport_refusesARangeRunningPastTheEnd(t *testing.T) {
	// with the total withheld the parser cannot catch this, so the body's own length has to
	body := &resumableBody{t: newTestTransport(), total: 100, offset: 50, done: make(chan struct{})}

	resp := &http.Response{
		StatusCode:    http.StatusPartialContent,
		ContentLength: 60,
		Header:        http.Header{"Content-Range": []string{"bytes 50-109/*"}},
		Body:          io.NopCloser(strings.NewReader("")),
	}

	_, err := body.acceptable(resp)
	assert.ErrorIs(t, err, errResumeRefused)
	assert.Contains(t, err.Error(), "past the end")
}

func TestResumableTransport_holdsAnUnsizedSegmentToItsSpan(t *testing.T) {
	// with no Content-Length the span cannot be checked up front, so the body has to stop at it
	body := &resumableBody{t: newTestTransport(), total: 100, offset: 50, done: make(chan struct{})}

	resp := &http.Response{
		StatusCode:    http.StatusPartialContent,
		ContentLength: -1,
		Header:        http.Header{"Content-Range": []string{"bytes 50-59/100"}},
		Body:          io.NopCloser(strings.NewReader(strings.Repeat("x", 30))),
	}

	segment, err := body.acceptable(resp)
	require.NoError(t, err)

	got, err := io.ReadAll(segment)
	require.NoError(t, err)
	assert.Len(t, got, 10, "only the declared span may be spliced in")
}

func TestResumableTransport_retriesAResumeTheServerCouldNotServeYet(t *testing.T) {
	// a CDN edge unhealthy enough to sever the stream is exactly the one liable to answer the
	// reopen with a 503, and that is no reason to discard everything already delivered
	payload := payloadOfSize(8192)
	server := &flakyBlobServer{
		payload:         payload,
		dropsLeft:       1,
		bytesPerDrop:    1000,
		transientStatus: http.StatusServiceUnavailable,
		transientTimes:  2,
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, got, "the blob must be reassembled across the refusals")

	// the initial request, the two refusals, and the attempt that finally served the range
	assert.Len(t, server.seenRanges(), 4)
}

func TestResumableTransport_classifiesARefusedResume(t *testing.T) {
	// a busy server spends the stall budget and is tried again; anything else is permanent and costs
	// one attempt. a 401 or 403 is permanent because the reopen client has already re-authorized and
	// followed a fresh redirect to get it, and a 307 here is one the client could not follow
	tests := []struct {
		status    int
		permanent bool
	}{
		{status: http.StatusRequestTimeout},
		{status: http.StatusTooManyRequests},
		{status: statusClientClosedRequest},
		{status: http.StatusBadGateway},
		{status: http.StatusServiceUnavailable},
		{status: http.StatusGatewayTimeout},
		{status: http.StatusOK, permanent: true},
		{status: http.StatusTemporaryRedirect, permanent: true},
		{status: http.StatusBadRequest, permanent: true},
		{status: http.StatusUnauthorized, permanent: true},
		{status: http.StatusForbidden, permanent: true},
		{status: http.StatusNotFound, permanent: true},
		{status: http.StatusGone, permanent: true},
		{status: http.StatusRequestedRangeNotSatisfiable, permanent: true},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			server := &flakyBlobServer{
				payload:         payloadOfSize(8192),
				dropsLeft:       1,
				bytesPerDrop:    1000,
				transientStatus: tt.status,
			}

			srv := httptest.NewServer(server)
			t.Cleanup(srv.Close)

			resp, err := newTestClient().Get(srv.URL + blobPath)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			_, err = io.ReadAll(resp.Body)
			require.Error(t, err)

			if tt.permanent {
				assert.ErrorIs(t, err, errResumeRefused)
				assert.Len(t, server.seenRanges(), 2, "a permanent refusal costs one attempt")
				return
			}

			assert.NotErrorIs(t, err, errResumeRefused, "a busy server must not be reported as a refusal")
			assert.Contains(t, err.Error(), "without progress")
			assert.Len(t, server.seenRanges(), maxConsecutiveStalls+1, "the whole stall budget is spent")
		})
	}
}

func Test_forLog(t *testing.T) {
	original := httptest.NewRequest(http.MethodGet, "https://registry.example.com/v2/img/blobs/sha256:abc", nil)
	cdn := httptest.NewRequest(http.MethodGet, "https://cdn.example.com/token=sekrit/blob?sig=sekrit", nil)
	cdn.Response = &http.Response{StatusCode: http.StatusTemporaryRedirect, Request: original}

	assert.Equal(t, "registry.example.com/v2/img/blobs/sha256:abc", forLog(cdn),
		"a signed CDN URL must never reach a log line or an error")

	// a foreign layer URL comes from the manifest, so its path is whatever the image author chose
	foreign := httptest.NewRequest(http.MethodGet, "https://evil.example/%1b[2J%0aFAKE/blobs/sha256:00", nil)
	assert.Equal(t, "evil.example/%1b[2J%0aFAKE/blobs/sha256:00", forLog(foreign),
		"a path that decodes to control characters must stay escaped")
}

func TestResumableTransport_acceptsAnIdentityContentEncoding(t *testing.T) {
	// reopen asks for identity, and a server that echoes the Accept-Encoding it was sent returns
	// it. identity is not a valid content-coding for a response, so it means the same as no header
	// at all; refusing it would permanently poison resuming against such a server.
	payload := payloadOfSize(8192)
	server := &flakyBlobServer{
		payload:         payload,
		dropsLeft:       1,
		bytesPerDrop:    1000,
		contentEncoding: "identity",
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

func Test_identityEncoded(t *testing.T) {
	encoded := func(values ...string) http.Header {
		return http.Header{"Content-Encoding": values}
	}

	assert.True(t, identityEncoded(http.Header{}))
	assert.True(t, identityEncoded(encoded("")))
	assert.True(t, identityEncoded(encoded("identity")))
	assert.True(t, identityEncoded(encoded("Identity")))
	assert.True(t, identityEncoded(encoded("  identity  ")))
	assert.True(t, identityEncoded(encoded("identity", "identity")))
	assert.False(t, identityEncoded(encoded("gzip")))
	assert.False(t, identityEncoded(encoded("identity, gzip")))
	assert.False(t, identityEncoded(encoded("identity", "gzip")), "a later coding still applies")
}

func TestResumableTransport_givesUpOnceTheTotalBudgetIsSpent(t *testing.T) {
	// every attempt delivers enough to replenish the stall budget, so only the total one can end
	// this. a path that drops this often is broken rather than flaky, and failing loudly is the
	// behaviour stereoscope had before this transport existed.
	payload := payloadOfSize(4 * 1024 * 1024)
	server := &flakyBlobServer{payload: payload, dropsLeft: 1000, bytesPerDrop: minResumeProgress}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "in total")

	// the initial request plus exactly the total budget, and no more
	assert.Len(t, server.seenRanges(), maxTotalResumes+1)
}

func TestResumableTransport_retriesABusyServerWhoseErrorPageIsEncoded(t *testing.T) {
	// the coding check guards a body about to be spliced, which is only ever a 206. a busy server
	// serving a compressed error page must still be retried, not refused as one that cannot resume
	payload := payloadOfSize(8192)
	server := &flakyBlobServer{
		payload:                  payload,
		dropsLeft:                1,
		bytesPerDrop:             1000,
		transientStatus:          http.StatusServiceUnavailable,
		transientTimes:           2,
		transientContentEncoding: "gzip",
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, got, "a compressed error page must not be read as a refusal to resume")

	// the initial request, the two refusals, and the attempt that finally served the range
	assert.Len(t, server.seenRanges(), 4)
}

func TestResumableTransport_closeAbortsAReopenInFlight(t *testing.T) {
	// closing a response body to abandon a read is documented usage, and an unwrapped net/http body
	// unblocks the reader at once. a reopen that has not yet returned is the one place the wrapper
	// could not reach by closing the current body, since there is no body yet.
	payload := payloadOfSize(2 * 1024 * 1024)

	var once sync.Once
	reopening := make(chan struct{})
	served := 0
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		served++
		first := served == 1
		mu.Unlock()

		if first {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload[:1024*1024])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}

		// accept the reopen and fall silent, never sending headers
		once.Do(func() { close(reopening) })
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	// a single stall allowed, so a Close mistaken for a failed attempt would exhaust the budget and
	// report giving up instead of the Close
	transport := newTestTransport()
	transport.maxStalls = 1

	resp, err := (&http.Client{Transport: transport}).Get(srv.URL + blobPath)
	require.NoError(t, err)

	read := make(chan error, 1)
	go func() {
		_, readErr := io.ReadAll(resp.Body)
		read <- readErr
	}()

	<-reopening
	require.NoError(t, resp.Body.Close())

	select {
	case readErr := <-read:
		assert.ErrorIs(t, readErr, http.ErrBodyReadAfterClose)
		assert.NotContains(t, readErr.Error(), "attempts", "a Close is not a failed attempt")
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not release a reader blocked on a reopen that never answered")
	}
}

func TestResumableTransport_cancellingThePullAbortsAReopenInFlight(t *testing.T) {
	// the caller's context ends a reopen the same way Close does, and is as final
	payload := payloadOfSize(2 * 1024 * 1024)
	reopening := make(chan struct{})
	var once sync.Once

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			// accept the reopen and fall silent, never sending headers
			once.Do(func() { close(reopening) })
			<-r.Context().Done()
			return
		}

		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload[:1024*1024])
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(srv.Close)

	transport := newTestTransport()
	transport.maxStalls = 1

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+blobPath, nil)
	require.NoError(t, err)

	resp, err := (&http.Client{Transport: transport}).Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	read := make(chan error, 1)
	go func() {
		_, readErr := io.ReadAll(resp.Body)
		read <- readErr
	}()

	<-reopening
	cancel()

	select {
	case readErr := <-read:
		assert.ErrorIs(t, readErr, context.Canceled)
		assert.NotContains(t, readErr.Error(), "attempts", "a cancellation is not a failed attempt")
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling the pull did not release a reader blocked on a reopen")
	}
}

func TestResumableTransport_aSilentReopenTimesOutWithoutLookingCancelled(t *testing.T) {
	// the header timeout ends a reopen by cancelling its context, but nobody cancelled the pull. a
	// caller sorting errors by errors.Is(err, context.Canceled) must not read this as an abort
	payload := payloadOfSize(64 * 1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			// accept the reopen and fall silent, never sending headers
			<-r.Context().Done()
			return
		}

		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload[:1024])
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(srv.Close)

	transport := newTestTransport()
	transport.idleTimeout = 50 * time.Millisecond

	resp, err := (&http.Client{Transport: transport}).Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no response to the resumed range request")
	assert.NotErrorIs(t, err, context.Canceled, "a timeout must not be reported as a cancellation")
	assert.Contains(t, err.Error(), "without progress", "a timeout is retried like any other stall")
}

func TestResumableTransport_resumesABodyThatFallsSilent(t *testing.T) {
	// a degraded CDN edge may keep the connection open and simply stop sending. without a clock on
	// the read that blocks until the caller's context ends, which may be never
	payload := payloadOfSize(8192)

	var mu sync.Mutex
	served := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		served++
		silent := served <= 2
		mu.Unlock()

		start, status := int64(0), http.StatusOK
		if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
			start, _ = parseRangeRequest(rangeHeader)
			status = http.StatusPartialContent
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)-int(start)))
		w.WriteHeader(status)

		// the first body and the first resumed segment both stall partway, so the clock has to be on
		// a resumed segment as well as on the original body
		if silent {
			_, _ = w.Write(payload[start : start+1000])
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		_, _ = w.Write(payload[start:])
	}))
	t.Cleanup(srv.Close)

	transport := newTestTransport()
	transport.idleTimeout = 200 * time.Millisecond

	resp, err := (&http.Client{Transport: transport}).Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	type result struct {
		got []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		got, readErr := io.ReadAll(resp.Body)
		done <- result{got, readErr}
	}()

	select {
	case r := <-done:
		require.NoError(t, r.err)
		assert.Equal(t, payload, r.got)
	case <-time.After(5 * time.Second):
		t.Fatal("a body that fell silent was never resumed")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 3, served, "both silent segments should have been abandoned and resumed")
}

func TestResumableTransport_aBodyThatStaysSilentSaysSo(t *testing.T) {
	// closing a silent body makes the read fail with whatever net/http reports for a closed body,
	// which would read as the caller having closed it
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "8192")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	transport := newTestTransport()
	transport.idleTimeout = 200 * time.Millisecond
	transport.maxStalls = 0

	resp, err := (&http.Client{Transport: transport}).Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data received within")
	assert.NotErrorIs(t, err, http.ErrBodyReadAfterClose)
}

func TestResumableTransport_aSlowReaderIsNotASilentServer(t *testing.T) {
	// the clock runs while a read waits on the server, not between reads, so a consumer that pauses
	// longer than the timeout must not cost a reconnect
	server := &flakyBlobServer{payload: payloadOfSize(8192)}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	transport := newTestTransport()
	transport.idleTimeout = 20 * time.Millisecond

	resp, err := (&http.Client{Transport: transport}).Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	_, err = io.ReadFull(resp.Body, make([]byte, 100))
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Len(t, server.seenRanges(), 1, "a pause on the reading side must not reopen the blob")
}

func TestResumableTransport_progressEarnsBackTotalBudget(t *testing.T) {
	// a long transfer that drops now and then, delivering plenty between drops, must not be held to
	// the same count as one that drops every few kilobytes
	payload := payloadOfSize(4 * 1024 * 1024)
	drops := maxTotalResumes + 10
	server := &flakyBlobServer{payload: payload, dropsLeft: drops, bytesPerDrop: minResumeProgress}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	transport := newTestTransport()
	transport.resumeCredit = minResumeProgress

	resp, err := (&http.Client{Transport: transport}).Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	assert.Len(t, server.seenRanges(), drops+1)
}

func TestResumableTransport_refusesASegmentLongerThanTheRangeItClaims(t *testing.T) {
	// a server with no real range support may echo back the range it was asked for and then serve
	// the object from the beginning anyway. it contradicts itself doing so (the body is longer
	// than the span the Content-Range claims), and that is worth catching here, because otherwise
	// the whole layer downloads again before the digest check rejects it.
	payload := payloadOfSize(8192)
	server := &flakyBlobServer{
		payload:        payload,
		dropsLeft:      1,
		bytesPerDrop:   1000,
		restartOnRange: true,
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	got, err := io.ReadAll(resp.Body)
	require.Error(t, err)
	assert.ErrorIs(t, err, errResumeRefused)
	assert.Contains(t, err.Error(), "covers")
	assert.Len(t, got, 1000, "only the bytes from before the drop are handed back")

	// permanent, so it costs one attempt rather than the whole budget
	assert.Len(t, server.seenRanges(), 2)
}

// clampingBlobServer honours every Range but answers with a self-consistent 206 far shorter than
// the span asked for: Content-Range, Content-Length and the bytes written all agree, so the segment
// is accepted and then ends in a bare io.EOF well short of the object. It is the shape a badly
// configured mirror takes, and the only one that produces an io.EOF as the cause of a terminal
// failure: an ordinary truncation is reported by net/http as io.ErrUnexpectedEOF already.
type clampingBlobServer struct {
	payload []byte
	clamp   int
}

func (s *clampingBlobServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := int64(0)
	status := http.StatusOK

	if spec := r.Header.Get("Range"); spec != "" {
		var end int64
		if _, err := fmt.Sscanf(spec, "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		status = http.StatusPartialContent

		sending := int64(s.clamp)
		if start+sending > int64(len(s.payload)) {
			sending = int64(len(s.payload)) - start
		}

		w.Header().Set("Content-Range",
			fmt.Sprintf("bytes %d-%d/%d", start, start+sending-1, len(s.payload)))
		w.Header().Set("Content-Length", strconv.FormatInt(sending, 10))
		w.WriteHeader(status)
		_, _ = w.Write(s.payload[start : start+sending])

		return
	}

	// the first response declares the whole object and delivers a fraction of it
	w.Header().Set("Content-Length", strconv.Itoa(len(s.payload)))
	w.WriteHeader(status)
	_, _ = w.Write(s.payload[:s.clamp])
	w.(http.Flusher).Flush()
}

func TestResumableTransport_aFailedTransferIsNeverACleanEndOfStream(t *testing.T) {
	// the sharpest edge this transport could leave: a caller testing errors.Is(err, io.EOF) to mean
	// "the stream ended" would read a truncated layer as a complete one. file.IterateTar in this
	// repo does exactly that test, so the terminal error must never carry io.EOF in its chain
	payload := payloadOfSize(256 * 1024)
	server := &clampingBlobServer{payload: payload, clamp: 4096}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := newTestClient().Get(srv.URL + blobPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	_, err = io.ReadAll(resp.Body)
	require.Error(t, err, "a transfer that never completes must fail")

	assert.NotErrorIs(t, err, io.EOF,
		"a truncation must not be mistakable for a clean end of stream")
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF,
		"it should be reported as the short read it is")
}

// acceptAndCloseListener serves one truncated blob over a raw socket and then answers every later
// connection by reading the request and closing without a response. That is what a load balancer
// or CDN edge whose backend pool has drained does, and net/http reports it to the caller of
// RoundTrip as a bare io.EOF. This is the second door into the terminal error, which httptest cannot
// produce because it always writes a response.
type acceptAndCloseListener struct {
	listener net.Listener
	payload  []byte
	prefix   int

	mu       sync.Mutex
	attempts int
}

func newAcceptAndCloseListener(t *testing.T, payload []byte, prefix int) *acceptAndCloseListener {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &acceptAndCloseListener{listener: listener, payload: payload, prefix: prefix}
	go server.serve()

	t.Cleanup(func() { _ = listener.Close() })

	return server
}

func (s *acceptAndCloseListener) url() string {
	return "http://" + s.listener.Addr().String() + blobPath
}

func (s *acceptAndCloseListener) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}

		go s.handle(conn)
	}
}

func (s *acceptAndCloseListener) handle(conn net.Conn) {
	defer conn.Close()

	s.mu.Lock()
	s.attempts++
	attempt := s.attempts
	s.mu.Unlock()

	// read the request line and headers, so the client has genuinely written its request
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}
	}

	if attempt > 1 {
		// accept, read, and close with no response at all
		return
	}

	fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", len(s.payload))
	_, _ = conn.Write(s.payload[:s.prefix])
}

func TestResumableTransport_aReopenThatIsNeverAnsweredIsNotACleanEndOfStream(t *testing.T) {
	// the sibling of aFailedTransferIsNeverACleanEndOfStream, and the door it does not cover: there
	// the io.EOF comes from the body read, here it comes from RoundTrip itself. the invariant is the
	// same and has to hold on both, or a caller testing errors.Is(err, io.EOF) reads a layer we
	// failed to fetch as one that simply ended
	payload := payloadOfSize(64 * 1024)
	server := newAcceptAndCloseListener(t, payload, 1024)

	transport := newTestTransport()

	resp, err := (&http.Client{Transport: transport}).Get(server.url())
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	got, err := io.ReadAll(resp.Body)
	require.Error(t, err, "a transfer whose every reopen goes unanswered must fail")

	assert.Len(t, got, 1024, "the bytes that did arrive should still be handed over")
	assert.NotErrorIs(t, err, io.EOF,
		"a reopen that was never answered must not be mistakable for a clean end of stream")
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF,
		"it should be reported as the short read it is")
}
