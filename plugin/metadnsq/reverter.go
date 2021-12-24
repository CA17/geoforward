package metadnsq

import (
	"github.com/miekg/dns"
)

type ResponseReverter struct {
	dns.ResponseWriter
	removeEcs bool
}

func NewResponseReverter(w dns.ResponseWriter) *ResponseReverter {
	return &ResponseReverter{
		ResponseWriter: w,
	}
}

func (r *ResponseReverter) Write(buf []byte) (int, error) {
	n, err := r.ResponseWriter.Write(buf)
	return n, err
}

// WriteMsg records the status code and calls the underlying ResponseWriter's WriteMsg method.
func (r *ResponseReverter) WriteMsg(res1 *dns.Msg) error {
	// Deep copy 'res' as to not (e.g). rewrite a message that's also stored in the cache.
	res := res1.Copy()
	if r.removeEcs {
		removeECS(res)
	}
	return r.ResponseWriter.WriteMsg(res)
}
