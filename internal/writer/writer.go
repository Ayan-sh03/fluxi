package writer

type Writer interface {
	Write(jobId, rootUrl, url, data string) error
}

type WriterImpl struct{}
