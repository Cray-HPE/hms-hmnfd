// MIT License
//
// (C) Copyright [2018, 2021, 2025] Hewlett Packard Enterprise Development LP
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
	"os"
	"os/signal"
	"syscall"
	"strconv"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/sirupsen/logrus"
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
	GroupId        string
}

// Kafka bus descriptor

type MsgBusWriter_Kafka struct {
	kafkaHost       string
	kafkaPort       int
	kafkaProducer   *kafka.Producer
	kafkaTopic      string

	//the following need to be in each msgbus type
	blocking       BlockingMode
	status         BusStatus
	connectRetries int
	handleLock     *sync.Mutex
}

type MsgBusReader_Kafka struct {
	kafkaHost     string
	kafkaPort     int
	kafkaConsumer *kafka.Consumer
	kafkaTopic    string
	kafkaGroupId  string
	cbfunc        CBFunc
	readQ         chan string

	//the following need to be in each msgbus type
	blocking       BlockingMode
	status         BusStatus
	connectRetries int
	handleLock     *sync.Mutex
}

// Other bus descriptors as they arise...

var logger = logrus.New()

//testing stuff

var __testmode bool = false
var __read_inject string


/****************************************************************************
 *           G E N E R A L  F U N C T I O N S
 ***************************************************************************/

func SetLogger(loggerP *logrus.Logger) {
	if (loggerP != nil) {
		logger = loggerP
	} else {
		logger = logrus.New()
	}
}

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

	mbus.handleLock.Lock()
	defer mbus.handleLock.Unlock()

	if (mbus.status != StatusOpen) {
		err = fmt.Errorf("ERROR: Attempting MessageWrite() on a closed connection.")
		return err
	}

	if (__testmode) {
		logger.Infof("TestMode: Sending message: %s",msg)
		return nil
	}

	if (mbus.blocking == Blocking) {
		deliveryChan := make(chan kafka.Event)

		err = mbus.kafkaProducer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &mbus.kafkaTopic, Partition: kafka.PartitionAny},
			Value:          []byte(msg),
		}, deliveryChan)

		e := <-deliveryChan
		m := e.(*kafka.Message)

		if m.TopicPartition.Error != nil {
			logger.Errorf("Kafka message delivery failed: %v\n", m.TopicPartition.Error)
		} else {
			logger.Infof("Delivered message to topic %s [%d] at offset %v\n",
				*m.TopicPartition.Topic, m.TopicPartition.Partition,
				m.TopicPartition.Offset)
		}

		close(deliveryChan)
	} else {
		//Writes message into kafka lib's Q.
		mbus.kafkaProducer.ProduceChannel() <- &kafka.Message{TopicPartition: kafka.TopicPartition{Topic: &mbus.kafkaTopic, Partition: kafka.PartitionAny}, Value: []byte(msg)}
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
	mbus.handleLock.Lock()
	defer mbus.handleLock.Unlock()

	if !__testmode {
		if ((mbus != nil) && (mbus.kafkaProducer != nil)) {
			mbus.kafkaProducer.Close()
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
	logger.Errorf("ERROR: MessageAvailable() not implemented for Writer interface.")
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
	mbus.handleLock.Lock()
	defer mbus.handleLock.Unlock()

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
	if (mbus.status == StatusClosed) {
		return 0
	}
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

func readerThread_Kafka(mbusP *MsgBusReader_Kafka) {
	var msg string
	var ev kafka.Event

	ok := true
	sigchan := make(chan os.Signal,1)
	signal.Notify(sigchan,syscall.SIGINT,syscall.SIGTERM)

	for {
		//Catch signals, and don't block
		select {
			case sig := <-sigchan:
				logger.Infof("Caught signal %v: terminating kafka read thread.",sig)
				mbusP.handleLock.Lock()
				mbusP.status = StatusClosed
				mbusP.handleLock.Unlock()
				ok = false
			default:
				_ = 1
		}

		msg = ""
		if (mbusP.status == StatusClosed) {
			break
		}

		if __testmode {
			msg = __read_inject
			time.Sleep(time.Millisecond * 10)
			if __read_inject == "" {
				ok = false
			}
		} else {
			logger.Tracef("About to poll...")
			mbusP.handleLock.Lock()
			if (mbusP.status != StatusClosed) {
				//Only wait a short time, then check, to prevent un-killable
				//hang.
				ev = mbusP.kafkaConsumer.Poll(500)	//block until message ready
			}
			mbusP.handleLock.Unlock()
			logger.Tracef("After poll.")
			if (ev == nil) {
				//Poll expired, nothing to do
				continue
			}

			switch e := ev.(type) {
				case *kafka.Message:
					msg = string(e.Value)

				case *kafka.Error:
					logger.Errorf("ERROR reading from kafka consumer: %v: %v",
						e.Code(), e)
					if (e.Code() == kafka.ErrAllBrokersDown) {
						//lost connection, go away
						ok = false
					}

				default:
					logger.Infof("Kafka poll() info: %v",e)
			}
		}

		if (!ok) {
			break
		}
		if (msg != "") {
			if (mbusP.cbfunc != nil) {
				mbusP.cbfunc(msg)
			} else {
				mbusP.readQ <- msg
			}
		}
	}
	close(mbusP.readQ)
}

//This thread only reports event info, has nothing to do with message
//delivery.

func writerThread_Kafka(mbusP *MsgBusWriter_Kafka) {
	for {
		mbusP.handleLock.Lock()
		if (mbusP.status == StatusClosed) {
			mbusP.handleLock.Unlock()
			return
		}
		evChan := mbusP.kafkaProducer.Events()
		mbusP.handleLock.Unlock()
		e,isOpen := <-evChan
		if (!isOpen) {
			logger.Infof("Closing writer thread, producer event chan is closed.")
			return
		}

		logger.Tracef("Got writer event: %v",e)

		switch ev := e.(type) {
			case *kafka.Message:
				m := ev
				if m.TopicPartition.Error != nil {
					logger.Errorf("Kafka message delivery failed: key: '%s', topic: '%s', partition: %d, error: %v",
						string(m.Key),
						*m.TopicPartition.Topic,
						m.TopicPartition.Partition,
						m.TopicPartition.Error)
				} else {
					logger.Tracef("Kafka delivered message to topic %s [%d] at offset %v",
						*m.TopicPartition.Topic, m.TopicPartition.Partition,
						m.TopicPartition.Offset)
				}

			default:
				logger.Tracef("Kafka producer: Ignored event: %s", ev)
		}
	}
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
	//
	//Note that if the chan is closed that means the reader thread is gone.
	//Reading this chan will always return an empty string, and will never
	//block.

	msg := ""
	if (mbus.status != StatusClosed) {
		msg = <-mbus.readQ
	}

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
// separated out from the connectWriter_Kafka() function to facilitate any
// future re-connect logic.
//
// kbus(in): Pointer to a Kafka writer descriptor.
// Return:   Error status of the connection operation.
/////////////////////////////////////////////////////////////////////////////

func connect_kafka_w(kbus *MsgBusWriter_Kafka) error {
	var err error = nil
	var ntry int

	broker := kbus.kafkaHost + ":" + strconv.Itoa(kbus.kafkaPort)

	for ntry = 0; ntry < kbus.connectRetries; ntry++ {
		if !__testmode {
			kbus.kafkaProducer,err = kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": broker})
		} else {
			err = nil
		}
		if (err != nil) {
			logger.Errorf("ERROR: Unable to connect to Kafka: %v Trying again in 1 second...",
				err)
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}
	if (ntry >= kbus.connectRetries) {
		return fmt.Errorf("ERROR, exhausted retry count (%d), cannot connect to Kafka bus.\n",
			kbus.connectRetries)
	}

	kbus.status = StatusOpen

	if (kbus.blocking == NonBlocking) {
		//This thread handles message error tracking for NB writers.
		go writerThread_Kafka(kbus)
	}

	logger.Printf("Connected to Confluent Kafka server as Writer.")
	return nil
}

/////////////////////////////////////////////////////////////////////////////
// Connect to a Kafka bus as a writer.  The passed-in config struct describes
// the connection parameters.
//
// cfg(in):  Connection parameters.
// Return:   Kafka writer descriptor/interface, error status of the operation.
/////////////////////////////////////////////////////////////////////////////

func connectWriter_Kafka(cfg MsgBusConfig) (*MsgBusWriter_Kafka, error) {
	var err error

	kbus := &MsgBusWriter_Kafka{status: StatusClosed,
		blocking:       cfg.Blocking,
		kafkaHost:      cfg.Host,
		kafkaPort:      cfg.Port,
		kafkaTopic:     cfg.Topic,
		connectRetries: cfg.ConnectRetries,
		handleLock:     &sync.Mutex{},
	}

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
	var consumer *kafka.Consumer
	var ntry int
	var err error = nil

	brokers := kbus.kafkaHost + ":" + strconv.Itoa(kbus.kafkaPort)
	topic := []string{kbus.kafkaTopic}

	if kbus.kafkaGroupId == "" {
		rnum := time.Now().Nanosecond()
		kbus.kafkaGroupId = fmt.Sprintf("%d", rnum)
	}

	for ntry = 0; ntry < kbus.connectRetries; ntry++ {
		if !__testmode {
			//TODO: consume-from-latest doesn't seem to work
			consumer, err = kafka.NewConsumer(&kafka.ConfigMap{
								"bootstrap.servers": brokers,
								"broker.address.family": "v4",
								"group.id": kbus.kafkaGroupId,
								"session.timeout.ms": 10000,
							    "auto.offset.reset": "latest"})
								//TODO: "default.topic.config": kafka.ConfigMap{"auto.offset.reset":"latest"},
		} else {
			err = nil
		}
		if err != nil {
			logger.Errorf("ERROR: Unable to connect to Kafka: '%v' - Trying again in 1 second...", err)
			time.Sleep(1 * time.Second)
			continue
		}

		//Got here, all is well.
		break
	}

	if ntry >= kbus.connectRetries {
		return fmt.Errorf("ERROR, exhausted retry count (%d), cannot connect to Kafka bus.\n",
			kbus.connectRetries)
	}

	if (!__testmode) {
		err = consumer.SubscribeTopics(topic,nil)
		if (err != nil) {
			return fmt.Errorf("Can't subscribe to topic '%s': %v",topic[0],err)
		}
	}

	kbus.kafkaConsumer = consumer
	kbus.status = StatusOpen
	logger.Printf("Connected to Confluent Kafka server as Reader.")
	return nil
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
		kafkaGroupId:   cfg.GroupId,
		connectRetries: cfg.ConnectRetries,
		handleLock:     &sync.Mutex{},
	}

	// Try to connect to Kafka...

	err = connect_kafka_r(kbus)
	if err == nil {
		kbus.readQ = make(chan string, MSG_QUEUE_MAX_LEN)
		go readerThread_Kafka(kbus)
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

	if (cfg.BusTech == 0) {
		err = fmt.Errorf("ERROR: Missing bus technology.")
		return badbus, err
	}
	if (cfg.Topic == "") {
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

	logger.Printf("Message bus connect, kafka, confluent interface.")

	if bcfg.ConnectRetries == 0 {
		bcfg.ConnectRetries = 1000000 //retry "forever"
	}

	if bcfg.BusTech == BusTechKafka {
		if bcfg.Direction == BusWriter {
			mbw, mbw_err := connectWriter_Kafka(bcfg)
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

