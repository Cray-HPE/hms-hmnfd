// MIT License
//
// (C) Copyright [2018, 2021] Hewlett Packard Enterprise Development LP
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

package msgbus

import (
	"fmt"
	"github.com/Shopify/sarama"
	"github.com/bsm/sarama-cluster"
	"strconv"
	"time"
)

/////////////////////////////////////////////////////////////////////////////
// Typedefs
/////////////////////////////////////////////////////////////////////////////

type MsgBusTech int
type BlockingMode int
type BusStatus int
type BusDir int
type SubscriptionToken int
type CallbackToken int
type CBFunc func(msg string)

/////////////////////////////////////////////////////////////////////////////
// Constants
/////////////////////////////////////////////////////////////////////////////

const (
	BusTechKafka MsgBusTech = 1
	//Add more if need be
)

const (
	NonBlocking BlockingMode = 1
	Blocking    BlockingMode = 2
)

const (
	StatusOpen   BusStatus = 1
	StatusClosed BusStatus = 2
)

const (
	BusWriter BusDir = 1
	BusReader BusDir = 2
)

const MSG_QUEUE_MAX_LEN = 1000

/////////////////////////////////////////////////////////////////////////////
// Structures and Interfaces
/////////////////////////////////////////////////////////////////////////////

// Message bus interface

type MsgBusIO interface {
	Disconnect() error
	MessageWrite(msg string) error
	MessageRead() (string, error)
	MessageAvailable() int //check for availability
	RegisterCB(cbfunc CBFunc) error
	UnregisterCB() error
	Status() int
}

// Generic bus parameters

type MsgBusConfig struct {
	BusTech        MsgBusTech
	Host           string
	Port           int
	Blocking       BlockingMode
	Direction      BusDir
	ConnectRetries int
	Topic          string
}

// Kafka bus descriptor

type MsgBusWriter_Kafka struct {
	kafkaHost       string
	kafkaPort       int
	kafkaConfig     *sarama.Config
	kafkaBProducer  *sarama.SyncProducer
	kafkaNBProducer *sarama.AsyncProducer
	kafkaTopic      string

	//the following need to be in each msgbus type
	blocking       BlockingMode
	status         BusStatus
	connectRetries int
}

type MsgBusReader_Kafka struct {
	kafkaHost     string
	kafkaPort     int
	kafkaConfig   *cluster.Config
	kafkaConsumer *cluster.Consumer
	kafkaTopic    string
	cbfunc        CBFunc
	readQ         chan string

	//the following need to be in each msgbus type
	blocking       BlockingMode
	status         BusStatus
	connectRetries int
}

// Other bus descriptors as they arise...

//testing stuff

var __testmode bool = false
var __read_inject string

/****************************************************************************
             K A F K A  B U S  F U N C T I O N S
****************************************************************************/

/////////////////////////////////////////////////////////////////////////////
//////////////////////////////  WRITERS /////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// Interface method, returns current status of Kafka bus connection.
//
// Return:   StatusOpen or StatusClosed
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusWriter_Kafka) Status() int {
	return int(mbus.status)
}

/////////////////////////////////////////////////////////////////////////////
// Interface method, injects a message string into the Kafka bus.  If the
// bus connection was opened up as a blocking connection, messages are
// injected directly, and can potentially block.  If opened as non-blocking,
// the message is placed on a queue to be processed by a goroutine.
//
// msg(in): Message to inject.
// Return:  Error data if there was an error.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusWriter_Kafka) MessageWrite(msg string) error {
	var err error = nil

	if mbus.status != StatusOpen {
		err = fmt.Errorf("ERROR: Attempting MessageWrite() on a closed connection.")
		return err
	}

	kmsg := &sarama.ProducerMessage{Topic: mbus.kafkaTopic,
		Value:     sarama.StringEncoder(msg),
		Timestamp: time.Now(),
	}

	if __testmode {
		fmt.Println("TestMode: Sending message:", kmsg)
	} else {
		if mbus.blocking == NonBlocking {
			(*mbus.kafkaNBProducer).Input() <- kmsg
			select {
			case err := <-(*mbus.kafkaNBProducer).Errors():
				fmt.Println("ERROR on async write channel:", err)
			default:
				err = nil
			}
		} else {
			_, _, err = (*mbus.kafkaBProducer).SendMessage(kmsg)
		}
	}

	return err
}

/////////////////////////////////////////////////////////////////////////////
// Disconnect from Kafka bus.
//
// Args:   None
// Return: Error data if there was an error.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusWriter_Kafka) Disconnect() error {
	if !__testmode {
		if mbus.blocking == NonBlocking {
			(*mbus.kafkaNBProducer).Close()
		} else {
			(*mbus.kafkaBProducer).Close()
		}
	}
	mbus.status = StatusClosed

	return nil
}

/////////////////////////////////////////////////////////////////////////////
// Interface method for registering a callback function.  Illegal for
// writers, but needed to fill out the interface specification.
//
// cbfunc(in): Ptr to a callback function.
// Return:     Error -- this is illegal, just a place holder.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusWriter_Kafka) RegisterCB(cbfunc CBFunc) error {
	return fmt.Errorf("ERROR: RegisterCB() not implemented for Writer interface.")
}

/////////////////////////////////////////////////////////////////////////////
// Interface method for un-registering a callback function.  Illegal for
// writers, but needed to fill out the interface specification.
//
// Return:     Error -- this is illegal, just a place holder.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusWriter_Kafka) UnregisterCB() error {
	return fmt.Errorf("ERROR: UnregisterCB() not implemented for Writer interface.")
}

/////////////////////////////////////////////////////////////////////////////
// Interface method for checking message read-availability. Illegal for
// writers, but needed to fill out the interface specification.
//
// Return: 0.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusWriter_Kafka) MessageAvailable() int {
	fmt.Print("ERROR: MessageAvailable() not implemented for Writer interface.")
	return 0
}

/////////////////////////////////////////////////////////////////////////////
// Interface method for blocking-read of inbound messagess.  Illegal for
// writers, but needed to fill out the interface specification.
//
// Return:  Empty string, Error -- this is illegal, just a place holder.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusWriter_Kafka) MessageRead() (string, error) {
	return "", fmt.Errorf("ERROR: MessageRead() not implemented for Writer interface.")
}

/////////////////////////////////////////////////////////////////////////////
////////////////////////// READERS //////////////////////////////////////////
/////////////////////////////////////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////
// Interface method for disconnecting a kafka bus reader connection.
//
// Return: Error status of operation.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusReader_Kafka) Disconnect() error {
	mbus.status = StatusClosed
	if !__testmode {
		(*mbus.kafkaConsumer).Close()
	}
	return nil
}

/////////////////////////////////////////////////////////////////////////////
// Interface method for checking reader connection status.
//
// Return: Status -- open or closed.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusReader_Kafka) Status() int {
	return int(mbus.status)
}

/////////////////////////////////////////////////////////////////////////////
// Interface method for registering a received-message callback function.
// This function is called whenever a message arrives from the subscribed
// topic.  This allows for 'event driven' programming models.
//
// cbfunc(in):  Function to call when messages arrive.
// Return:      Error status of operation.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusReader_Kafka) RegisterCB(cbfunc CBFunc) error {
	if mbus.cbfunc != nil {
		err := fmt.Errorf("ERROR: callback function already registered.")
		return err
	}

	mbus.cbfunc = cbfunc
	return nil
}

/////////////////////////////////////////////////////////////////////////////
// Interface method for un-registering a callback function.
//
// Return: Error status of operation. (currently can't fail)
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusReader_Kafka) UnregisterCB() error {
	mbus.cbfunc = nil
	return nil
}

/////////////////////////////////////////////////////////////////////////////
// Interface function for checking for message availability on a reader
// connection.  Note that the behavior of this function is undefined if
// a callback function has been registered; it is only valuable to use
// for blocking read connections.
//
//Return: 0 if no messages are available, > 0 == message(s) available.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusReader_Kafka) MessageAvailable() int {
	return len(mbus.readQ)
}

/////////////////////////////////////////////////////////////////////////////
// Thread function which monitors incoming messages.  It does blocking reads
// from a Kafka bus, and then dispatches received messages to either an
// internal queue (for blocking reads) or to a registered callback function
// (for event driven models).  This thread will exit when the connection is
// closed intentionally, or if for whatever reason the Kafka read channel
// closes.  TODO: at some point we may need some re-connection logic.
//
// mbusP(in): Pointer to a kafka reader descriptor from Connect().
// Return:    None.
/////////////////////////////////////////////////////////////////////////////

func ReaderThread_Kafka(mbusP *MsgBusReader_Kafka) {
	var msg *sarama.ConsumerMessage
	var err error
	var ok bool

	for {
		err = nil
		if mbusP.status == StatusClosed {
			break
		}

		if __testmode {
			msg = &sarama.ConsumerMessage{Key: []byte("TestKey"),
				Value:     []byte(__read_inject),
				Topic:     "TestTopic",
				Partition: 0,
				Offset:    1234}
			time.Sleep(time.Millisecond * 10)
			if __read_inject == "" {
				ok = false
			} else {
				ok = true
			}
		} else {
			msg, ok = <-(*mbusP.kafkaConsumer).Messages()
			select {
			case cerr := <-(*mbusP.kafkaConsumer).Errors():
				err = fmt.Errorf("%s", cerr.Error())
				fmt.Println("ERROR reading consumer channel:", err)
			default:
				err = nil
			}
		}
		if !ok {
			break //channel went away, time to die
		}

		if err != nil {
			continue
		}

		if mbusP.cbfunc != nil {
			mbusP.cbfunc(string(msg.Value))
		} else {
			if !__testmode {
				(*mbusP.kafkaConsumer).MarkOffset(msg, "ok")
			}

			mbusP.readQ <- string(msg.Value)
		}
	}
	close(mbusP.readQ)
}

/////////////////////////////////////////////////////////////////////////////
// Interface method to return the next message available on a reader.  There
// may be > 1, which would require another call to this func.  This function
// will block if there are no messages available.  To check before reading,
// call MessageAvailable().
//
// Return:  Message string received and error status of operation.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusReader_Kafka) MessageRead() (string, error) {
	if mbus.cbfunc != nil {
		err := fmt.Errorf("ERROR: Invalid to call MessageRead(), callback is registered.")
		return "", err
	}

	//Block if there are no messages available.  Note that we're not reading
	//from the Kafka interface, but via an internal chan.  This is because
	//there is no $%#!! way to check message availability with this interface.
	//Thus, we have to use a goroutine to slurp the messages off and queue
	//them up, then check the chan for availability.

	msg := <-mbus.readQ
	return msg, nil
}

/////////////////////////////////////////////////////////////////////////////
// Interface function to write a message to the kafka bus.  The topic was
// determined when the connection was opened.
//
// msg(in):  Message string to write.
// Return:   Error status of the operation.
/////////////////////////////////////////////////////////////////////////////

func (mbus *MsgBusReader_Kafka) MessageWrite(msg string) error {
	return fmt.Errorf("ERROR: MessageWrite not implemented for Reader interface.")
}

/////////////////////////////////////////////////////////////////////////////
// Convenience function to open a Kafka bus writer connection.  This was
// separated out from the ConnectWriter_Kafka() function to facilitate any
// future re-connect logic.
//
// kbus(in): Pointer to a Kafka writer descriptor.
// Return:   Error status of the connection operation.
/////////////////////////////////////////////////////////////////////////////

func connect_kafka_w(kbus *MsgBusWriter_Kafka) error {
	var err error = nil
	var ntry int

	brokers := []string{kbus.kafkaHost + ":" + strconv.Itoa(kbus.kafkaPort)}

	if kbus.blocking == Blocking {
		var producer sarama.SyncProducer

		//Needed for sync producer
		kbus.kafkaConfig.Producer.Return.Successes = true
		kbus.kafkaConfig.Producer.Return.Errors = true

		for ntry = 0; ntry < kbus.connectRetries; ntry++ {
			if !__testmode {
				producer, err = sarama.NewSyncProducer(brokers, kbus.kafkaConfig)
			} else {
				err = nil
			}
			if err != nil {
				fmt.Println("ERROR: Unable to connect to Kafka! Trying again in 1 second...",
					err)
				time.Sleep(1 * time.Second)
			} else {
				break
			}
		}
		if ntry >= kbus.connectRetries {
			err = fmt.Errorf("ERROR, exhausted retry count (%d), cannot connect to Kafka bus.\n",
				kbus.connectRetries)
			return err
		}

		kbus.kafkaBProducer = &producer
	} else {
		var producer sarama.AsyncProducer

		for ntry = 0; ntry < kbus.connectRetries; ntry++ {
			if !__testmode {
				producer, err = sarama.NewAsyncProducer(brokers, kbus.kafkaConfig)
			} else {
				err = nil
			}
			if err != nil {
				fmt.Println("ERROR: Unable to connect to Kafka! Trying again in 1 second...")
				time.Sleep(1 * time.Second)
			} else {
				break
			}
		}

		if ntry >= kbus.connectRetries {
			err = fmt.Errorf("ERROR, exhausted retry count (%d), cannot connect to Kafka bus.\n",
				kbus.connectRetries)
			return err
		}

		kbus.kafkaNBProducer = &producer
	}

	kbus.status = StatusOpen
	fmt.Println("Connected to Kafka server as Writer.")
	return nil
}

/////////////////////////////////////////////////////////////////////////////
// Connect to a Kafka bus as a writer.  The passed-in config struct describes
// the connection parameters.
//
// cfg(in):  Connection parameters.
// Return:   Kafka writer descriptor/interface, error status of the operation.
/////////////////////////////////////////////////////////////////////////////

func ConnectWriter_Kafka(cfg MsgBusConfig) (*MsgBusWriter_Kafka, error) {
	var err error

	kbus := &MsgBusWriter_Kafka{status: StatusClosed,
		blocking:       cfg.Blocking,
		kafkaHost:      cfg.Host,
		kafkaPort:      cfg.Port,
		kafkaTopic:     cfg.Topic,
		connectRetries: cfg.ConnectRetries,
	}

	kbus.kafkaConfig = sarama.NewConfig()
	kbus.kafkaConfig.Version = sarama.V0_10_0_0
	kbus.kafkaConfig.Producer.RequiredAcks = sarama.WaitForLocal
	kbus.kafkaConfig.Producer.Compression = sarama.CompressionNone
	kbus.kafkaConfig.Producer.Retry.Max = 10

	// Try to connect to Kafka...

	err = connect_kafka_w(kbus)
	return kbus, err
}

/////////////////////////////////////////////////////////////////////////////
// Convenience function to open a Kafka bus reader connection.  This was
// separated out from the ConnectReader_Kafka() function to facilitate any
// future re-connect logic.
//
// kbus(in): Pointer to a Kafka reader descriptor.
// Return:   Error status of the connection operation.
/////////////////////////////////////////////////////////////////////////////

func connect_kafka_r(kbus *MsgBusReader_Kafka) error {
	var consumer *cluster.Consumer
	var client *cluster.Client
	var ntry int
	var err error = nil

	brokers := []string{kbus.kafkaHost + ":" + strconv.Itoa(kbus.kafkaPort)}
	topic := []string{kbus.kafkaTopic}

	rnum := time.Now().Nanosecond()
	gid := fmt.Sprintf("%d", rnum)

	for ntry = 0; ntry < kbus.connectRetries; ntry++ {
		//NOTE: could just use cluster.NewConsumer(), but exposing the client
		//interface has advantages for things like getting message offsets,
		//partition info, etc.
		if !__testmode {
			client, err = cluster.NewClient(brokers, kbus.kafkaConfig)
		} else {
			err = nil
		}
		if err != nil {
			fmt.Println("ERROR: can't create new kafka client: Trying again in 1 second...", err)
			time.Sleep(time.Second)
			continue
		}
		if !__testmode {
			consumer, err = cluster.NewConsumerFromClient(client, gid, topic)
		} else {
			err = nil
		}
		if err != nil {
			fmt.Println("ERROR: Unable to connect to Kafka: Trying again in 1 second...", err)
			time.Sleep(1 * time.Second)
			continue
		}

		//Got here, all is well.
		break
	}

	if ntry >= kbus.connectRetries {
		err = fmt.Errorf("ERROR, exhausted retry count (%d), cannot connect to Kafka bus.\n",
			kbus.connectRetries)
		return err
	}

	//TEST CODE, DON'T DELETE

	//parts,perr := client.Partitions(kbus.kafkaTopic)
	//if (perr != nil) {
	//    fmt.Println("ERROR getting partitions:",perr)
	//} else {
	//    fmt.Println("Partitions:",parts)
	//    for i := range(parts) {
	//        off,err := client.GetOffset(kbus.kafkaTopic,int32(i),sarama.OffsetNewest)
	//        if (err != nil) {
	//            fmt.Println("ERROR getting offset:",err)
	//        } else {
	//            fmt.Printf("Partition %d offset: %d\n",i,off)
	//        }
	//    }
	//}

	kbus.kafkaConsumer = consumer

	kbus.status = StatusOpen
	fmt.Println("Connected to Kafka server as Reader.")
	return err
}

/////////////////////////////////////////////////////////////////////////////
// Connect to a Kafka bus as a reader.  The passed-in config struct describes
// the connection parameters.
//
// cfg(in):  Connection parameters.
// Return:   Kafka reader descriptor/interface, error status of the operation.
/////////////////////////////////////////////////////////////////////////////

func ConnectReader_Kafka(cfg MsgBusConfig) (*MsgBusReader_Kafka, error) {
	var err error

	kbus := &MsgBusReader_Kafka{status: StatusClosed,
		kafkaHost:      cfg.Host,
		kafkaPort:      cfg.Port,
		kafkaTopic:     cfg.Topic,
		connectRetries: cfg.ConnectRetries,
	}

	kbus.kafkaConfig = cluster.NewConfig()
	kbus.kafkaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest

	// Try to connect to Kafka...

	err = connect_kafka_r(kbus)
	if err == nil {
		kbus.readQ = make(chan string, MSG_QUEUE_MAX_LEN)
		go ReaderThread_Kafka(kbus)
	}
	return kbus, err
}

/////////////////////////////////////////////////////////////////////////////
// Connect to a message bus.  This function is not part of the interface,
// but will return the correct struct for the given interface.
//
// NOTE: this is one-way at a time for now.  Bus connection can be a reader
// or a writer, but not both.  That is a potential future enhancement.  If
// an application needs both, 2 connections must be made.
//
// cfg(in):  Connection parameters.
// Return:   MsgBusIO interface, and error status
/////////////////////////////////////////////////////////////////////////////

func Connect(cfg MsgBusConfig) (MsgBusIO, error) {
	var err error = nil
	badbus := &MsgBusWriter_Kafka{}
	bcfg := cfg

	//Check for missing stuff

	if cfg.BusTech == 0 {
		err = fmt.Errorf("ERROR: Missing bus technology.")
		return badbus, err
	}
	if cfg.Topic == "" {
		err = fmt.Errorf("ERROR: Missing bus topic.")
		return badbus, err
	}
	//Fixup defaults

	if bcfg.Host == "" {
		bcfg.Host = "localhost"
	}
	if bcfg.Port == 0 {
		bcfg.Port = 9092
	}
	switch bcfg.Blocking {
	case 0:
		bcfg.Blocking = NonBlocking
	case NonBlocking:
	case Blocking:
		bcfg.Blocking = cfg.Blocking //need a NOP...
	default:
		err = fmt.Errorf("ERROR: Bad 'Blocking' value: %d", int(cfg.Blocking))
		return badbus, err
	}
	if bcfg.Direction == 0 {
		err = fmt.Errorf("ERROR: must specify bus direction.")
		return badbus, err
	} else if (bcfg.Direction != BusWriter) && (bcfg.Direction != BusReader) {
		err = fmt.Errorf("ERROR: invalid bus direction '%d'", int(bcfg.Direction))
		return badbus, err
	}
	if bcfg.ConnectRetries == 0 {
		bcfg.ConnectRetries = 1000000 //retry "forever"
	}

	if bcfg.BusTech == BusTechKafka {
		if bcfg.Direction == BusWriter {
			mbw, mbw_err := ConnectWriter_Kafka(bcfg)
			return mbw, mbw_err
		} else {
			mbr, mbr_err := ConnectReader_Kafka(bcfg)
			return mbr, mbr_err
		}
	}

	//More bus technologies here...

	//If we got here, it's an error.

	err = fmt.Errorf("ERROR: Unknown message bus type '%d'.\n",
		int(bcfg.BusTech))
	kb := &MsgBusWriter_Kafka{}
	return kb, err
}
