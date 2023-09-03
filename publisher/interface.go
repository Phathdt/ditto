package publisher

import "ditto/models"

type Publisher interface {
	Publish(topic string, event models.Event) error
}
