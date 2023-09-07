// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package omsnozzle

import (
	"encoding/json"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/vmware-tanzu/nozzle-for-microsoft-azure-log-analytics/caching"
	"github.com/vmware-tanzu/nozzle-for-microsoft-azure-log-analytics/client"
	"github.com/vmware-tanzu/nozzle-for-microsoft-azure-log-analytics/firehose"
	"github.com/vmware-tanzu/nozzle-for-microsoft-azure-log-analytics/messages"
)

type ProcessedMessage struct {
	msgType string
	data    interface{}
}

type OmsNozzle struct {
	logger              lager.Logger
	maxCCGoroutines     int
	msgChan             chan *events.Envelope
	processedMessages   chan ProcessedMessage
	signalChan          chan os.Signal
	omsClient           client.Client
	firehoseClient      firehose.Client
	nozzleConfig        *NozzleConfig
	goroutineSem        chan int // to control the number of active post goroutines
	cachingClient       caching.CachingClient
	totalEventsReceived uint64
	totalEventsSent     uint64
	totalEventsLost     uint64
	totalDataSent       uint64
	totalEventsDropped  uint64
	mutex               *sync.Mutex
}

type NozzleConfig struct {
	OmsTypePrefix         string
	OmsBatchTime          time.Duration
	OmsMaxMsgNumPerBatch  int
	ExcludeMetricEvents   bool
	ExcludeLogEvents      bool
	ExcludeHttpEvents     bool
	LogEventCount         bool
	LogEventCountInterval time.Duration
}

func NewOmsNozzle(logger lager.Logger, firehoseClient firehose.Client, omsClient client.Client, nozzleConfig *NozzleConfig, caching caching.CachingClient) *OmsNozzle {
	maxPostGoroutines := int(100000 / nozzleConfig.OmsMaxMsgNumPerBatch)
	return &OmsNozzle{
		logger:              logger,
		msgChan:             make(chan *events.Envelope, 1000),
		processedMessages:   make(chan ProcessedMessage, 1000),
		signalChan:          make(chan os.Signal, 2),
		omsClient:           omsClient,
		firehoseClient:      firehoseClient,
		nozzleConfig:        nozzleConfig,
		goroutineSem:        make(chan int, maxPostGoroutines),
		maxCCGoroutines:     maxPostGoroutines / 10,
		cachingClient:       caching,
		totalEventsReceived: uint64(0),
		totalEventsSent:     uint64(0),
		totalEventsLost:     uint64(0),
		totalDataSent:       uint64(0),
		totalEventsDropped:  uint64(0),
		mutex:               &sync.Mutex{},
	}
}

func (o *OmsNozzle) Start() error {
	o.cachingClient.Initialize()

	// setup for termination signal from CF
	signal.Notify(o.signalChan, syscall.SIGTERM, syscall.SIGINT)
	go o.readEnvelopes()
	for i := 0; i <= o.maxCCGoroutines; i++ {
		//this should also be refactored
		go o.processEnvelopes()
	}
	if o.nozzleConfig.LogEventCount {
		o.logTotalEvents(o.nozzleConfig.LogEventCountInterval)
	}
	err := o.routeEvents()
	return err
}

func (o *OmsNozzle) readEnvelopes() {
	msgChan, errChan := o.firehoseClient.Connect()
	for {
		select {
		case msg := <-msgChan:
			select {
			case o.msgChan <- msg:
			default:
				o.totalEventsDropped = o.totalEventsDropped + 1
				if o.totalEventsDropped%1000 == 0 {
					o.logger.Error("dropping messages", nil, lager.Data{"total dropped": o.totalEventsDropped})
				}
			}
		case err := <-errChan:
			o.logger.Error("Error while reading from the firehose", err)

			if strings.Contains(err.Error(), "close 1008 (policy violation)") {
				o.logger.Error("Disconnected because nozzle couldn't keep up. Please try scaling up the nozzle.", nil)
				o.logSlowConsumerAlert()
			}

			o.logger.Error("Closing connection with traffic controller", nil)
			o.firehoseClient.CloseConsumer() //nolint:errcheck
			msgChan, errChan = o.firehoseClient.Connect()
		}
	}
}

func (o *OmsNozzle) processEnvelopes() {
	for {
		msg := <-o.msgChan
		atomic.AddUint64(&o.totalEventsReceived, 1)
		// process message
		var omsMessageType = msg.GetEventType().String()
		switch msg.GetEventType() {
		// Metrics
		case events.Envelope_ValueMetric:
			if !o.nozzleConfig.ExcludeMetricEvents {
				omsMessage := messages.NewValueMetric(msg, o.cachingClient)
				o.processedMessages <- ProcessedMessage{msgType: omsMessageType, data: omsMessage}
			}
		case events.Envelope_CounterEvent:
			m := messages.NewCounterEvent(msg, o.cachingClient)
			if strings.Contains(m.Name, "TruncatingBuffer.DroppedMessage") {
				o.logger.Error("received TruncatingBuffer alert", nil)
				o.logSlowConsumerAlert()
			}
			if strings.Contains(m.Name, "doppler_proxy.slow_consumer") && m.Delta > 0 {
				o.logger.Error("received slow_consumer alert", nil)
				o.logSlowConsumerAlert()
			}
			if !o.nozzleConfig.ExcludeMetricEvents {
				omsMessage := m
				o.processedMessages <- ProcessedMessage{msgType: omsMessageType, data: omsMessage}
			}

		case events.Envelope_ContainerMetric:
			if !o.nozzleConfig.ExcludeMetricEvents {
				omsMessage := messages.NewContainerMetric(msg, o.cachingClient)
				if omsMessage != nil { //nolint:staticcheck
					o.processedMessages <- ProcessedMessage{msgType: omsMessageType, data: omsMessage}
				}
			}

		// Logs Errors
		case events.Envelope_LogMessage:
			if !o.nozzleConfig.ExcludeLogEvents {
				omsMessage := messages.NewLogMessage(msg, o.cachingClient)
				if omsMessage != nil {
					o.processedMessages <- ProcessedMessage{msgType: omsMessageType, data: omsMessage}
				}
			}

		case events.Envelope_Error:
			if !o.nozzleConfig.ExcludeLogEvents {
				omsMessage := messages.NewError(msg, o.cachingClient)
				o.processedMessages <- ProcessedMessage{msgType: omsMessageType, data: omsMessage}
			}

		// HTTP Start/Stop
		case events.Envelope_HttpStartStop:
			if !o.nozzleConfig.ExcludeHttpEvents {
				omsMessage := messages.NewHTTPStartStop(msg, o.cachingClient)
				if omsMessage != nil {
					o.processedMessages <- ProcessedMessage{msgType: omsMessageType, data: omsMessage}
				}
			}
		default:
			o.logger.Info("uncategorized message", lager.Data{"message": msg.String()})
		}
	}

}

func (o *OmsNozzle) logTotalEvents(interval time.Duration) {
	logEventCountTicker := time.NewTicker(interval)
	lastReceivedCount := uint64(0)
	lastSentCount := uint64(0)
	lastLostCount := uint64(0)
	lastDroppedCount := uint64(0)

	go func() {
		for range logEventCountTicker.C {
			timeStamp := time.Now().UnixNano()
			totalReceivedCount := atomic.LoadUint64(&o.totalEventsReceived)
			totalSentCount := atomic.LoadUint64(&o.totalEventsSent)
			totalLostCount := atomic.LoadUint64(&o.totalEventsLost)
			totalDroppedCount := atomic.LoadUint64(&o.totalEventsDropped)
			currentEvents := make(map[string][]interface{})

			// Generate CounterEvent
			o.addEventCountEvent("eventsReceived", totalReceivedCount-lastReceivedCount, totalReceivedCount, &timeStamp, &currentEvents)
			o.addEventCountEvent("eventsSent", totalSentCount-lastSentCount, totalSentCount, &timeStamp, &currentEvents)
			o.addEventCountEvent("eventsLost", totalLostCount-lastLostCount, totalLostCount, &timeStamp, &currentEvents)
			o.addEventCountEvent("eventsDropped", totalDroppedCount-lastDroppedCount, totalDroppedCount, &timeStamp, &currentEvents)

			o.goroutineSem <- 1
			o.postData(&currentEvents, false)

			lastReceivedCount = totalReceivedCount
			lastSentCount = totalSentCount
			lastLostCount = totalLostCount
			lastDroppedCount = totalDroppedCount
		}
	}()
}

func (o *OmsNozzle) addEventCountEvent(name string, deltaCount uint64, count uint64, timeStamp *int64, currentEvents *map[string][]interface{}) {
	counterEvent := &events.CounterEvent{
		Name:  &name,
		Delta: &deltaCount,
		Total: &count,
	}

	eventType := events.Envelope_CounterEvent
	job := "nozzle"
	origin := "stats"
	envelope := &events.Envelope{
		EventType:    &eventType,
		Timestamp:    timeStamp,
		Job:          &job,
		Origin:       &origin,
		CounterEvent: counterEvent,
	}

	var omsMsg OMSMessage
	eventTypeString := eventType.String()
	omsMsg = messages.NewCounterEvent(envelope, o.cachingClient)
	(*currentEvents)[eventTypeString] = append((*currentEvents)[eventTypeString], omsMsg)
}

func (o *OmsNozzle) postData(events *map[string][]interface{}, addCount bool) {
	for k, v := range *events {
		v := v
		if len(v) <= 0 {
			continue
		}
		if msgAsJson, err := json.Marshal(&v); err != nil {
			o.logger.Error("error marshalling message to JSON", err,
				lager.Data{"event type": k},
				lager.Data{"event count": len(v)})
		} else {
			o.logger.Debug("Posting to OMS",
				lager.Data{"event type": k},
				lager.Data{"event count": len(v)},
				lager.Data{"total size": len(msgAsJson)})
			if len(o.nozzleConfig.OmsTypePrefix) > 0 {
				k = o.nozzleConfig.OmsTypePrefix + k
			}
			nRetries := 4
			for nRetries > 0 {
				requestStartTime := time.Now()
				if err = o.omsClient.PostData(&msgAsJson, k); err != nil {
					nRetries--
					elapsedTime := time.Since(requestStartTime)
					o.logger.Error("error posting message to OMS", err,
						lager.Data{"event type": k},
						lager.Data{"elapse time": elapsedTime.String()},
						lager.Data{"event count": len(v)},
						lager.Data{"total size": len(msgAsJson)},
						lager.Data{"remaining attempts": nRetries})
					time.Sleep(time.Second * 1)
				} else {
					if addCount {
						atomic.AddUint64(&o.totalEventsSent, uint64(len(v)))
						atomic.AddUint64(&o.totalDataSent, uint64(len(v)))
					}
					break
				}
			}
			if nRetries == 0 && addCount {
				atomic.AddUint64(&o.totalEventsLost, uint64(len(v)))
			}
		}
	}
	<-o.goroutineSem
}

func (o *OmsNozzle) routeEvents() error {
	pendingEvents := make(map[string][]interface{})
	// Firehose message processing loop
	ticker := time.NewTicker(o.nozzleConfig.OmsBatchTime)
	for {
		// loop over message and signal channel
		select {
		case s := <-o.signalChan:
			o.logger.Info("exiting", lager.Data{"signal caught": s.String()})
			err := o.firehoseClient.CloseConsumer()
			if err != nil {
				o.logger.Error("error closing consumer", err)
			}
			os.Exit(1)
		case <-ticker.C:
			// get the pending as current
			currentEvents := pendingEvents
			// reset the pending events
			pendingEvents = make(map[string][]interface{})
			o.goroutineSem <- 1
			go o.postData(&currentEvents, true)
		case msg := <-o.processedMessages:
			pendingEvents[msg.msgType] = append(pendingEvents[msg.msgType], msg.data)
			// When the number of one type of events reaches the max per batch, trigger the post immediately
			doPost := false
			for _, v := range pendingEvents {
				if len(v) >= o.nozzleConfig.OmsMaxMsgNumPerBatch {
					doPost = true
					break
				}
			}
			if doPost {
				currentEvents := pendingEvents
				pendingEvents = make(map[string][]interface{})
				o.goroutineSem <- 1
				go o.postData(&currentEvents, true)
			}
		}
	}
}

// Log slowConsumerAlert as a ValueMetric event to OMS
func (o *OmsNozzle) logSlowConsumerAlert() {
	name := "slowConsumerAlert"
	value := float64(1)
	unit := "b"
	valueMetric := &events.ValueMetric{
		Name:  &name,
		Value: &value,
		Unit:  &unit,
	}

	timeStamp := time.Now().UnixNano()
	eventType := events.Envelope_ValueMetric
	job := "nozzle"
	origin := "alert"
	envelope := &events.Envelope{
		EventType:   &eventType,
		Timestamp:   &timeStamp,
		Job:         &job,
		Origin:      &origin,
		ValueMetric: valueMetric,
	}

	omsMsg := messages.NewValueMetric(envelope, o.cachingClient)
	currentEvents := make(map[string][]interface{})
	currentEvents[eventType.String()] = append(currentEvents[eventType.String()], omsMsg)

	o.goroutineSem <- 1
	o.postData(&currentEvents, false)
}

// OMSMessage is a marker inteface for JSON formatted messages published to OMS
type OMSMessage interface{}
