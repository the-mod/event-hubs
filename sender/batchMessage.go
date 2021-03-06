package sender

import (
	"context"
	eventhub "github.com/Azure/azure-event-hubs-go/v3"
	"math"
	"runtime"
	"sync"
)

func (sender *Sender) triggerBatches(ctx context.Context, wg *sync.WaitGroup, numGoRoutines int, eventBatches map[int]*List) {
	wg.Add(len(eventBatches)) //the number of the event batches must to be less or equals to numGoRoutines.

	for j := 0; j < numGoRoutines; j++ {
		if j == len(eventBatches) {
			break
		}

		if sender.onBeforeSendBatchMessage != nil {
			batchTotalOfMessage := getAmountOfBatchMessages(eventBatches[j])
			sender.onBeforeSendBatchMessage(batchTotalOfMessage, j)
		}
		go sendBatchMessages(sender, eventBatches[j], wg, ctx, j)
	}

	wg.Wait()
}

func getAmountOfBatchMessages(list *List) int {
	var totalOfMessages int

	if list != nil && list.Size() > 0 {
		var batchSize = list.Size()

		for i := 0; i < batchSize; i++ {
			events, _ := list.Get(i)
			totalOfMessages += len(events)
		}
	}

	return totalOfMessages
}

func sendBatchMessages(sender *Sender, eventBatches *List, wg *sync.WaitGroup, ctx context.Context, workerIndex int) {
	defer func() {
		wg.Done()
		<- ctx.Done()
	}()

	if eventBatches != nil && eventBatches.Size() > 0 {
		batchSize := eventBatches.Size()
		var mutex sync.Mutex

		for i := 0; i < batchSize; i++ {
			events, _ := eventBatches.Get(i)
			runtime.Gosched()
			_ = sender.eHub.SendBatch(ctx, eventhub.NewEventBatchIterator(events...))

			if sender.onAfterSendBatchMessage != nil {
				mutex.Lock()
				sender.onAfterSendBatchMessage(len(events), workerIndex)
				mutex.Unlock()
			}
		}
	}
}

func createEventBatchCollection(sender *Sender, numGoRoutines int, limit int64, numMessages int64,
	message string, withSuffix bool) map[int]*List {

	var result = make(map[int]*List)
	var numOfBatches = int(math.Round(float64(numMessages / limit)))
	var messagesCounter int64 = 0

	for i := 0; i <= numOfBatches; i++ {
		for j := 0; j < numGoRoutines; j++ {
			if result[j] == nil {
				result[j] = New()
			}
			events := getEventsToBatch(limit, numMessages, message, sender.properties,	sender.base64String, withSuffix)
			result[j].Add(events)

			messagesCounter = messagesCounter + int64(len(events))
			if messagesCounter + limit >= numMessages {
				left := numMessages - messagesCounter
				if left > 0 {
					eventsLeft := getEventsToBatch(left, numMessages, message, sender.properties, sender.base64String,
						withSuffix)
					if eventsLeft != nil {
						result[len(result) - 1].Add(eventsLeft)
					}
				}

				return result
			}
		}
		i = i + (numGoRoutines - 1)
	}

	return result
}

func getEventsToBatch(limit int64, numMessages int64, message string, properties []string, base64 bool,
	withSuffix bool) []*eventhub.Event {

	var events []*eventhub.Event
	var event *eventhub.Event
	var d int64

	for d = 0; d < limit; d++ {
		//any change in the line bellow affect the limit calculation
		event = createAnEvent(base64, message, withSuffix)
		addProperties(event, properties)
		events = append(events, event)

		if int64(len(events)) == numMessages {
			break
		}
	}

	return events
}

func createEventBatchCollectionWithEvents(eventsSeed *[]*eventhub.Event, numGoRoutines int, limit int64,
	numMessages int64) map[int]*List {

	var result = make(map[int]*List)
	var numOfBatches = int(math.Round(float64(numMessages / limit)))
	var messagesCounter int64 = 0
	var offset int64 = 0

	for i := 0; i <= numOfBatches; i++ {
		for j := 0; j < numGoRoutines; j++ {
			if result[j] == nil {
				result[j] = New()
			}

			events := getEventsToBatchWithEvents(limit, numMessages, eventsSeed, offset)
			offset = offset + int64(len(events))
			if offset >= int64(len(*eventsSeed)) {
				offset = 0
			}
			result[j].Add(events)

			messagesCounter = messagesCounter + int64(len(events))
			if messagesCounter + limit >= numMessages {
				left := numMessages - messagesCounter
				if left > 0 {
					eventsLeft := getEventsToBatchWithEvents(left, numMessages, eventsSeed, offset)
					if eventsLeft != nil {
						result[len(result) - 1].Add(eventsLeft)
					}
				}

				return result
			}
		}
		i = i + (numGoRoutines - 1)
	}

	return result
}

func getEventsToBatchWithEvents(limit int64, numMessages int64, eventsSeed *[]*eventhub.Event, index int64) []*eventhub.Event {
	var events []*eventhub.Event
	var event *eventhub.Event
	var d int64
	var size = int64(len(*eventsSeed))

	for d = index; d < size; d++ {
		//any change in the line bellow affect the limit calculation
		event = (*eventsSeed)[d]
		events = append(events, event)

		eventsSize := int64(len(events))
		if eventsSize == numMessages || eventsSize == limit {
			break
		}
	}

	return events
}