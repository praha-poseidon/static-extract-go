package header

type Header struct{}

func (Header) Get() {}

type HeaderHandler struct{}

func (HeaderHandler) Run() {
	var header Header
	header.Get()
}
