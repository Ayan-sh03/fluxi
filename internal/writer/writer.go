package writer

type Writer interface {
	Write(url, data string) error
}

type WriterImpl struct{}
