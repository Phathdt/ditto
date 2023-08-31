package watermillapp

type Publisher interface {
	Publish(topic string, data interface{}) error
}
