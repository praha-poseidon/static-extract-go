package redis

type Client struct{}

func (Client) Get() {}

func Lookup() {}

type RedisHandler struct{}

func (RedisHandler) Run() {
	var client Client
	client.Get()
}
