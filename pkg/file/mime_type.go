package file

import (
	"io"
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

// MIMEType attempts to guess at the MIME type of a file given the contents. If there is no contents, then an empty
// string is returned. If the MIME type could not be determined and the contents are not empty, then a MIME type
// of "application/octet-stream" is returned.
func MIMEType(reader io.Reader) string {
	if reader == nil {
		return ""
	}
	var mTypeStr string
	mType, err := mimetype.DetectReader(reader)
	if err == nil {
		// extract the string mimetype and ignore aux information (e.g. 'text/plain; charset=utf-8' -> 'text/plain')
		mTypeStr = strings.Split(mType.String(), ";")[0]
	}
	return mTypeStr
}
