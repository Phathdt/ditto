package natsrouter

import (
	"context"
	"flag"
	"time"

	sctx "github.com/viettranx/service-context"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-jetstream/pkg/jetstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/ThreeDotsLabs/watermill/message/router/plugin"
	"github.com/nats-io/nats.go"
)

type NoPublishHandlerFunc func(msg *message.Message) error

type natsRouter struct {
	id         string
	natsURL    string
	router     *message.Router
	subscriber *jetstream.Subscriber
	logger     sctx.Logger
}

func NewNatsRouter(id string) *natsRouter {
	return &natsRouter{id: id}
}

func (n *natsRouter) ID() string {
	return n.id
}

func (n *natsRouter) InitFlags() {
	flag.StringVar(&n.natsURL, n.id+"-URI", "nats://localhost:4222", "nats uri")
}

func (n *natsRouter) Activate(sc sctx.ServiceContext) error {
	n.logger = sc.Logger(n.id)

	marshaler := &jetstream.GobMarshaler{}
	logger := watermill.NewStdLogger(false, false)
	options := []nats.Option{
		nats.RetryOnFailedConnect(true),
		nats.Timeout(30 * time.Second),
		nats.ReconnectWait(1 * time.Second),
	}
	subscribeOptions := []nats.SubOpt{
		nats.DeliverAll(),
		nats.AckExplicit(),
	}

	subscriber, err := jetstream.NewSubscriber(
		jetstream.SubscriberConfig{
			URL:              n.natsURL,
			QueueGroup:       "processor",
			CloseTimeout:     30 * time.Second,
			AckWaitTimeout:   30 * time.Second,
			NatsOptions:      options,
			Unmarshaler:      marshaler,
			SubscribeOptions: subscribeOptions,
			AutoProvision:    true,
		},
		logger,
	)
	if err != nil {
		return err
	}

	n.subscriber = subscriber

	router, err := message.NewRouter(message.RouterConfig{}, logger)
	if err != nil {
		panic(err)
	}

	router.AddPlugin(plugin.SignalsHandler)

	router.AddMiddleware(
		middleware.CorrelationID,

		middleware.Retry{
			MaxRetries:      3,
			InitialInterval: time.Millisecond * 100,
			Logger:          logger,
		}.Middleware,
		middleware.Recoverer,
	)

	n.router = router

	go func() {
		time.Sleep(5 * time.Second)

		ctx := context.Background()

		_ = n.router.Run(ctx)
	}()

	return nil
}

func (n *natsRouter) Stop() error {
	err := n.router.Close()
	if err != nil {
		n.logger.Error(err)
	}

	return err
}

func (n *natsRouter) AddNoPublisherHandler(handlerName string, subscribeTopic string, handlerFunc func(msg *message.Message) error) {
	n.router.AddNoPublisherHandler(handlerName, subscribeTopic, n.subscriber, handlerFunc)
}
