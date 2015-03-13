package http

import (
	"fmt"
	"github.com/mozilla-services/heka/message"
	p "github.com/mozilla-services/heka/pipeline"
	"io"
	"net/http"
)

func CustomHeadersHandler(h http.Handler, header http.Header) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, vals := range header {
			for _, val := range vals {
				w.Header().Add(key, val)
			}
		}
		h.ServeHTTP(w, r)
	})
}

func splitStream(ir p.InputRunner, sRunner p.SplitterRunner, r io.ReadCloser) error {
	var (
		record       []byte
		longRecord   []byte
		err          error
		deliver      bool
		nullSplitter bool
	)
	// If we're using a NullSplitter we want to make sure we capture the
	// entire HTTP request or response body and not be subject to what we get
	// from a single Read() call.
	if _, ok := sRunner.Splitter().(*p.NullSplitter); ok {
		nullSplitter = true
	}
	for err == nil {
		deliver = true
		_, record, err = sRunner.GetRecordFromStream(r)
		if err == io.ErrShortBuffer {
			if sRunner.KeepTruncated() {
				err = fmt.Errorf("record exceeded MAX_RECORD_SIZE %d and was truncated",
					message.MAX_RECORD_SIZE)
			} else {
				deliver = false
				err = fmt.Errorf("record exceeded MAX_RECORD_SIZE %d and was dropped",
					message.MAX_RECORD_SIZE)
			}
			ir.LogError(err)
			err = nil // non-fatal, keep going
		} else if sRunner.IncompleteFinal() && err == io.EOF && len(record) == 0 {
			record = sRunner.GetRemainingData()
		}
		if len(record) > 0 && deliver {
			if nullSplitter {
				// Concatenate all the records until EOF. This should be safe
				// b/c NullSplitter means FindRecord will always return the
				// full buffer contents, we don't have to worry about
				// GetRecordFromStream trying to append multiple reads to a
				// single record and triggering an io.ErrShortBuffer error.
				longRecord = append(longRecord, record...)
			} else {
				sRunner.DeliverRecord(record, nil)
			}
		}
	}
	r.Close()
	if err == io.EOF && nullSplitter && len(longRecord) > 0 {
		sRunner.DeliverRecord(longRecord, nil)
	}
	return err
}
