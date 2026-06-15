// eventtail subscribes to RabbitMQ events and prints them to stdout.
//
// Usage:
//
//	go run ./cmd/eventtail --url amqp://webitel:webitel@172.22.22.22:5672
//	go run ./cmd/eventtail --url amqp://... --filter bot-control
//	go run ./cmd/eventtail --url amqp://... --filter bot-control --thread <thread_id>
//
// Available filters: bot-control, messages, members, all
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	exchange     = "im_message.events"
	exchangeType = "topic"
)

var (
	green = "\033[32m"
	red   = "\033[31m"
	cyan  = "\033[36m"
	reset = "\033[0m"
	bold  = "\033[1m"
	dim   = "\033[2m"
)

var filters = map[string]string{
	"bot-control": "im_thread.*.bot.control.#",
	"messages":    "im_message.*.message.created.#",
	"members":     "im_thread.*.member.#",
	"all":         "im_thread.#",
}

var queuePrefixes = map[string]string{
	"bot-control": "eventtail.bot.control",
	"messages":    "eventtail.messages",
	"members":     "eventtail.members",
	"all":         "eventtail.all",
}

func queueName(filter string) string {
	host, _ := os.Hostname()
	if host == "" {
		host = "local"
	}
	return fmt.Sprintf("%s.%s", queuePrefixes[filter], host)
}

func colorFor(topic string) string {
	switch {
	case strings.Contains(topic, "granted"):
		return green
	case strings.Contains(topic, "released"):
		return red
	case strings.Contains(topic, "created"):
		return cyan
	default:
		return ""
	}
}

func shortTopic(topic string) string {
	parts := strings.Split(topic, ".")
	if len(parts) > 2 {
		return strings.Join(parts[2:], ".")
	}
	return topic
}

func printEvent(topic string, body []byte) {
	color := colorFor(topic)
	ts := time.Now().Format("15:04:05.000")

	fmt.Printf("\n%s%s[%s] %s%s\n", bold, color, ts, shortTopic(topic), reset)
	fmt.Printf("%stopic: %s%s\n", dim, topic, reset)

	var pretty map[string]any
	if err := json.Unmarshal(body, &pretty); err == nil {
		out, _ := json.MarshalIndent(pretty, "  ", "  ")
		fmt.Printf("  %s\n", string(out))
	} else {
		fmt.Printf("  %s\n", string(body))
	}
}

func main() {
	url := flag.String("url", "amqp://webitel:webitel@localhost:5673", "RabbitMQ URL")
	filter := flag.String("filter", "bot-control", "Event filter: bot-control | messages | members | all")
	threadID := flag.String("thread", "", "Narrow to specific thread_id (optional)")
	flag.Parse()

	routingKey, ok := filters[*filter]
	if !ok {
		log.Fatalf("unknown filter %q. Available: bot-control, messages, members, all", *filter)
	}
	queueName := queueName(*filter)

	// narrow to specific thread if provided
	if *threadID != "" {
		switch *filter {
		case "bot-control":
			routingKey = fmt.Sprintf("im_thread.%s.bot.control.#", *threadID)
		case "members":
			routingKey = fmt.Sprintf("im_thread.%s.member.#", *threadID)
		case "all":
			routingKey = fmt.Sprintf("im_thread.%s.#", *threadID)
		}
		fmt.Printf("Filter   : %s (thread %s)\n", *filter, *threadID)
	} else {
		fmt.Printf("Filter   : %s\n", *filter)
	}
	fmt.Printf("Exchange : %s\n", exchange)
	fmt.Printf("Queue    : %s\n", queueName)
	fmt.Printf("Routing  : %s\n\n", routingKey)

	conn, err := amqp.Dial(*url)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("channel: %v", err)
	}
	defer ch.Close()

	if err = ch.ExchangeDeclarePassive(exchange, exchangeType, true, false, false, false, nil); err != nil {
		ch2, _ := conn.Channel()
		if err2 := ch2.ExchangeDeclare(exchange, exchangeType, true, false, false, false, nil); err2 != nil {
			log.Fatalf("exchange declare: %v", err2)
		}
		ch2.Close()
		ch, _ = conn.Channel()
	}

	// exclusive=true: queue is auto-deleted when this connection closes
	q, err := ch.QueueDeclare(queueName, false, false, true, false, nil)
	if err != nil {
		log.Fatalf("queue declare: %v", err)
	}

	if err = ch.QueueBind(q.Name, routingKey, exchange, false, nil); err != nil {
		log.Fatalf("queue bind: %v", err)
	}

	deliveries, err := ch.Consume(q.Name, "eventtail", true, true, false, false, nil)
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	fmt.Println("Connected. Waiting for events...")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-sig:
			fmt.Println("\nStopped.")
			return
		case d, ok := <-deliveries:
			if !ok {
				fmt.Println("Channel closed.")
				return
			}
			printEvent(d.RoutingKey, d.Body)
		}
	}
}
