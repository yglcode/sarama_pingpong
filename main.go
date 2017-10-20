package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"
	"strings"
	"strconv"
	"math"

	"github.com/Shopify/sarama"
	"github.com/bsm/sarama-cluster"
)

// Sarama currently cannot support latest kafka protocol version 0_11_
var (
	SARAMA_KAFKA_PROTO_VER = sarama.V0_10_2_0
)

func main() {
	log.SetFlags(0)

	kafkaBrokers := []string{"kafka1:19092"}
	group := "kafka-sarama-pingpong"
	var pub_topics,sub_topics []string

	if val, exists := os.LookupEnv("kafka_brokers"); exists {
		kafkaBrokers = strings.Split(val,",")
		for i:=0;i<len(kafkaBrokers); {
			if len(kafkaBrokers[i])==0 {
				kafkaBrokers = append(kafkaBrokers[:i],kafkaBrokers[i+1:]...)
			} else {
				i++
			}
		}
	}

	if val, exists := os.LookupEnv("consumer_group"); exists {
		group = val
	}

	if val, exists := os.LookupEnv("pub_topics"); exists {
		pub_topics = strings.Split(val,",")
		for i:=0;i<len(pub_topics); {
			if len(pub_topics[i])==0 {
				pub_topics = append(pub_topics[:i],pub_topics[i+1:]...)
			} else {
				i++
			}
		}
	}

	if val, exists := os.LookupEnv("sub_topics"); exists {
		sub_topics = strings.Split(val,",")
		for i:=0;i<len(sub_topics); {
			if len(sub_topics[i])==0 {
				sub_topics = append(sub_topics[:i],sub_topics[i+1:]...)
			} else {
				i++
			}
		}
	}

	// Kafka-queue-worker and zookeeper & kafka brokers may start at same time
	// Wait for that kafka is up and topics are provisioned
	var alltops []string
	alltops = append(alltops,pub_topics...)
	alltops = append(alltops,sub_topics...)
	waitforBrokersTopics(kafkaBrokers,alltops)

	//setup consumer
	cConfig := cluster.NewConfig()
	cConfig.Version = SARAMA_KAFKA_PROTO_VER
	cConfig.Consumer.Return.Errors = true
	cConfig.Consumer.Offsets.Initial = sarama.OffsetOldest  //OffsetNewest
	cConfig.Group.Return.Notifications = true
	cConfig.Group.Session.Timeout = 6 * time.Second
	cConfig.Group.Heartbeat.Interval = 2 * time.Second

	consumer, err := cluster.NewConsumer(kafkaBrokers, group, sub_topics, cConfig)
	if err != nil {
		log.Fatalln("Fail to create Kafka consumer: ",err)
	}
	defer consumer.Close()

	fmt.Printf("Created Consumer %v\n", consumer)

	//setup async producer; should we use sync producer (lower throughput)?
	pConfig := sarama.NewConfig()
	pConfig.Version = SARAMA_KAFKA_PROTO_VER
	pConfig.Producer.Return.Successes = true
	producer, err := sarama.NewAsyncProducer(kafkaBrokers, pConfig)

	if err != nil {
		log.Fatalf("Fail to create KafkaQueue %s\n",err)
	}

	defer producer.Close()

	fmt.Printf("Created Producer %v\n", producer)


	//as Ponger, initiate the msg flow
	cnt := 1
	msgHead := "Hello Go!"
	value := fmt.Sprintf("%s %d",msgHead,cnt)
	if group == "ponger" {
		producer.Input() <- &sarama.ProducerMessage{Topic: pub_topics[0], Key: nil, Value: sarama.StringEncoder(value)}
		fmt.Println("ponger send first msg")
	}

	// Wait for a SIGINT (perhaps triggered by user with CTRL-C)
	// Run cleanup when signal is received
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	numClosedChans := 0
	for numClosedChans < 2 {
		select {
		case _ = <- signalChan:
			fmt.Printf("\nReceived an interrupt, exit...\n\n")
			// Trigger drain & close the following 2 chan
			producer.AsyncClose()
		case msg,ok := <-producer.Successes():
			if ok {
				fmt.Printf("%s: succeed sending 1 request: %v\n",group,msg)
			} else {
				numClosedChans++
			}
		case err, ok := <-producer.Errors():
			if ok {
				fmt.Println(group,": producer error: ",err)
			} else {
				numClosedChans++
			}
		case msg, ok := <-consumer.Messages():
			if ok {
				fmt.Printf("- %s recv on %v %v: %v\n", group, msg.Topic, msg.Partition, string(msg.Value))
				data := strings.Split(string(msg.Value)," ")
				cnt, err = strconv.Atoi(data[len(data)-1])
				if err != nil {
					fmt.Println("xxx String/Int conversion failure:",err)
					break
				}
				cnt = (cnt+1)%math.MaxInt32
				value = fmt.Sprintf("%s %d",msgHead,cnt)
				fmt.Printf("- %s send: %s\n", group, value)
				producer.Input() <- &sarama.ProducerMessage{Topic: pub_topics[0], Key: nil, Value: sarama.StringEncoder(value)}

				consumer.MarkOffset(msg, "") // mark message as processed
			}
		case err = <- consumer.Errors():
			fmt.Println("consumer error: ",err)
		case ntf := <- consumer.Notifications():
			fmt.Println("Rebalanced: %+v\n", ntf)
		}
	}
	fmt.Println("--------- ",group," exits ----------")
}


// OpenFaas gateway and zookeeper & kafka brokers may start at same time
// wait for that kafka is up and topics are provisioned
func waitforBrokersTopics(brokers []string, topics []string) {
	var client sarama.Client 
	var err error
	for {
		client,err = sarama.NewClient(brokers,nil)
		if client!=nil && err==nil { break }
		if client!=nil { client.Close() }
		fmt.Println("Wait for kafka brokers coming up...")
		time.Sleep(2*time.Second)
	}
	fmt.Println("Kafka brokers up")
LOOP_TOPIC:
	for {
		count := len(topics)
		tops,err := client.Topics()
		if tops!=nil && err==nil {
			for _,t1 := range tops {
				for _,t2 := range topics {
					if t1==t2 {
						fmt.Println("Topic ",t2," is ready")
						count--
						if count==0 { // All expected topics ready
							break LOOP_TOPIC
						} else {
							break
						}
					}
				}
			}
		}
		fmt.Println("Wait for topics:",topics,"...")
		client.RefreshMetadata(topics...)
		time.Sleep(2*time.Second)
	}
	client.Close()
}
