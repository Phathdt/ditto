package watermillapp

import "github.com/ThreeDotsLabs/watermill/message"

type Subscriber interface {
	AddNoPublisherHandler(handlerName string, subscribeTopic string, handlerFunc func(msg *message.Message) error)
}
