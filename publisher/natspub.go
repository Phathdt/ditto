package publisher

import (
	"github.com/ThreeDotsLabs/watermill-jetstream/pkg/jetstream"
	"github.com/sirupsen/logrus"
)

type natsPublisher struct {
	publisher *jetstream.Publisher
	logger    *logrus.Entry
}
