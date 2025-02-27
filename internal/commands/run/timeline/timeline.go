package timeline

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/artuross/github-actions-runner/internal/helpers/to"
	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
)

type ID string

type Type string

const (
	TypeJob  Type = "Job"
	TypeTask Type = "Task"
)

type State string

const (
	StateInProgress = "inProgress"
	StateCompleted  = "completed"
	StatePending    = "pending"
)

type Result string

const (
	ResultSucceeded Result = "succeeded"
)

type Update any

type LogUploaded struct {
	ID    ID
	LogID int64
}

type UpdateStarted struct {
	ID        ID
	StartTime time.Time
}

type UpdateFinished struct {
	ID         ID
	FinishTime time.Time
	Result     Result
}

var noopChan = make(<-chan time.Time)

type Log struct {
	ID       int
	Location *string
}

type Record struct {
	ID              ID
	ParentID        *ID
	Type            Type
	Name            string
	StartTime       *time.Time
	FinishTime      *time.Time
	PercentComplete int
	State           State
	Result          *Result
	ChangeID        int
	Order           int
	RefName         string
	Log             *Log
	Location        *string
	Identifier      *string
}

type RecordDiff struct {
	ID              ID
	ParentID        *ID
	Type            *Type
	Name            *string
	StartTime       *time.Time
	FinishTime      *time.Time
	PercentComplete int
	State           *State
	Result          *Result
	ChangeID        int
	Order           int
	RefName         string
	Log             *Log
	Location        *string
	Identifier      *string
}

type Controller struct {
	ghClient   *ghactions.Repository
	runnerName string
	planID     string
	timelineID string

	shutdownSignal chan struct{}
	wg             sync.WaitGroup
	queuedUpdates  []Update
	queueChan      chan Update
	localState     []Record
	serverState    []Record
}

func NewController(ghClient *ghactions.Repository, runnerName, planID, timelineID string) *Controller {
	return &Controller{
		ghClient:   ghClient,
		runnerName: runnerName,
		planID:     planID,
		timelineID: timelineID,

		shutdownSignal: make(chan struct{}),
		wg:             sync.WaitGroup{},
		queuedUpdates:  make([]Update, 0),
		queueChan:      make(chan Update),
		localState:     make([]Record, 0),
		serverState:    make([]Record, 0),
	}
}

func (c *Controller) Start(ctx context.Context) {
	// create a timer and stop it right away
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}

	// create an empty channel to read timer ticks
	// it will be used to throttle updates
	timerChan := noopChan

	tick := func() bool {
		if len(c.queuedUpdates) == 0 {
			timerChan = noopChan

			return false
		}

		c.tick(ctx)

		return true
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		for {
			select {
			case <-c.shutdownSignal:
				// stop the timer, we'll sync here
				if timerChan != noopChan && !timer.Stop() {
					<-timer.C
				}

				// drain the channel
			drain:
				for {
					select {
					case record := <-c.queueChan:
						c.queuedUpdates = append(c.queuedUpdates, record)
						continue

					default:
						break drain
					}
				}

				// do pending updates
				tick()

				return

			case record := <-c.queueChan:
				c.queuedUpdates = append(c.queuedUpdates, record)

				// wait for 100ms, as other updates will be likely queued
				if timerChan == noopChan {
					timer.Reset(100 * time.Millisecond)
					timerChan = timer.C
				}

			case <-timerChan:
				if !tick() {
					continue
				}

				// throttle
				timer.Reset(100 * time.Millisecond)
			}
		}
	}()
}

func (c *Controller) JobStarted(id string, displayName, refName string, startTime time.Time) {
	c.queueChan <- Record{
		ID:        ID(id),
		Type:      TypeJob,
		State:     StateInProgress,
		Name:      displayName,
		RefName:   refName,
		StartTime: &startTime,
	}
}

// TODO: add result
func (c *Controller) JobCompleted(id string, finishTime time.Time) {
	c.queueChan <- UpdateFinished{
		ID:         ID(id),
		FinishTime: finishTime,
		Result:     ResultSucceeded,
	}
}

func (c *Controller) AddRecord(id string, parentID string, name, refName string) {
	c.queueChan <- Record{
		ID:       ID(id),
		ParentID: to.Ptr(ID(parentID)),
		Type:     TypeTask,
		State:    StatePending,
		Name:     name,
		RefName:  refName,
	}
}

func (c *Controller) GetLogWriter(ctx context.Context, id string) io.WriteCloser {
	return NewLogWriter(ctx, c.ghClient, c, c.planID, id)
}

func (c *Controller) RecordLogUploaded(id string, logID int64) {
	c.queueChan <- LogUploaded{
		ID:    ID(id),
		LogID: logID,
	}
}

func (c *Controller) RecordStarted(id string, startTime time.Time) {
	c.queueChan <- UpdateStarted{
		ID:        ID(id),
		StartTime: startTime,
	}
}

func (c *Controller) RecordFinished(id string, finishTime time.Time) {
	c.queueChan <- UpdateFinished{
		ID:         ID(id),
		FinishTime: finishTime,
		Result:     ResultSucceeded,
	}
}

func (c *Controller) Shutdown(ctx context.Context) error {
	// close channel to notify goroutine it's time to exit
	close(c.shutdownSignal)

	// wait for goroutine to finish
	c.wg.Wait()

	return nil
}

func (c *Controller) tick(ctx context.Context) {
	for _, update := range c.queuedUpdates {
		switch update := update.(type) {
		case Record:
			// add new record to local state
			// TODO: put the record in right location (reorder)

			siblings := 0
			for _, item := range c.localState {
				if to.Value(item.ParentID) == to.Value(update.ParentID) {
					siblings++
				}
			}

			update.Order = siblings + 1

			c.localState = append(c.localState, update)

		case LogUploaded:
			recordIndex := slices.IndexFunc(c.localState, recordWithID(update.ID))

			if recordIndex == -1 {
				// TODO: handle
				fmt.Println("record not found")
				continue
			}

			record := c.localState[recordIndex]

			record.Log = &Log{
				ID: int(update.LogID),
			}

			c.localState[recordIndex] = record

		case UpdateFinished:
			recordIndex := slices.IndexFunc(c.localState, recordWithID(update.ID))

			if recordIndex == -1 {
				// TODO: handle
				fmt.Println("record not found")
				continue
			}

			record := c.localState[recordIndex]

			record.State = StateCompleted
			record.Result = &update.Result
			record.FinishTime = &update.FinishTime

			c.localState[recordIndex] = record

		case UpdateStarted:
			recordIndex := slices.IndexFunc(c.localState, recordWithID(update.ID))

			if recordIndex == -1 {
				// TODO: handle
				fmt.Println("record not found")
				continue
			}

			record := c.localState[recordIndex]

			record.State = StateInProgress
			record.StartTime = &update.StartTime

			c.localState[recordIndex] = record
		}
	}

	c.queuedUpdates = make([]Update, 0)

	// calculate diffs with server
	diffs := calculateDiffs(c.localState, c.serverState)
	if len(diffs) == 0 {
		return
	}

	clientDiffs := convertToAPIDiffs(diffs, c.runnerName)
	serverRecords, err := c.ghClient.PatchTimeline(ctx, c.planID, c.timelineID, clientDiffs)
	if err != nil {
		// TODO: handle error
		fmt.Println("patch", err)
		return
	}

	syncedRecords := make([]Record, 0, len(serverRecords))
	for _, sr := range serverRecords {
		var log *Log
		if sr.Log != nil {
			log = &Log{
				ID:       sr.Log.ID,
				Location: &sr.Log.Location,
			}
		}

		syncedRecords = append(syncedRecords, Record{
			ID:              ID(sr.ID),
			ParentID:        (*ID)(sr.ParentID),
			Type:            Type(sr.Type),
			Name:            sr.Name,
			StartTime:       sr.StartTime,
			FinishTime:      sr.FinishTime,
			PercentComplete: sr.PercentComplete,
			State:           State(sr.State),
			Result:          (*Result)(sr.Result),
			ChangeID:        sr.ChangeID,
			Order:           sr.Order,
			RefName:         sr.RefName,
			Log:             log,
			Location:        sr.Location,
			Identifier:      sr.Identifier,
		})
	}

	for _, record := range syncedRecords {
		recordIndex := slices.IndexFunc(c.serverState, recordWithID(record.ID))
		if recordIndex == -1 {
			c.serverState = append(c.serverState, record)
			continue
		}

		c.serverState[recordIndex] = record
	}

	for _, record := range syncedRecords {
		recordIndex := slices.IndexFunc(c.localState, recordWithID(record.ID))
		if recordIndex == -1 {
			c.localState = append(c.localState, record)
			continue
		}

		c.localState[recordIndex] = record
	}
}

func recordWithID(id ID) func(Record) bool {
	return func(record Record) bool {
		return record.ID == id
	}
}

func calculateDiffs(localState []Record, serverState []Record) []RecordDiff {
	diffs := make([]RecordDiff, 0)

	for _, local := range localState {
		serverIndex := slices.IndexFunc(serverState, recordWithID(local.ID))

		if serverIndex == -1 {
			diffs = append(diffs, convertRecordToRecordDiff(local))
			continue
		}

		diff, needsUpdate := getRecordDiff(local, serverState[serverIndex])
		if !needsUpdate {
			continue
		}

		diffs = append(diffs, diff)
	}

	return diffs
}

func convertRecordToRecordDiff(record Record) RecordDiff {
	return RecordDiff{
		ID:              record.ID,
		Type:            &record.Type,
		ParentID:        record.ParentID,
		Name:            &record.Name,
		StartTime:       record.StartTime,
		FinishTime:      record.FinishTime,
		PercentComplete: record.PercentComplete,
		State:           &record.State,
		Result:          record.Result,
		ChangeID:        record.ChangeID,
		Order:           record.Order,
		RefName:         record.RefName,
		Log:             record.Log,
		Location:        record.Location,
		Identifier:      record.Identifier,
	}
}

func getRecordDiff(local, server Record) (RecordDiff, bool) {
	diff := RecordDiff{
		ID:              local.ID,
		Type:            &local.Type,
		Name:            &local.Name,
		PercentComplete: local.PercentComplete,
		State:           &local.State,
		ChangeID:        local.ChangeID,
		Order:           local.Order,
		RefName:         local.RefName,
		Location:        local.Location,
		Identifier:      local.Identifier,
	}

	needsSync := false

	if local.ParentID != server.ParentID {
		diff.ParentID = local.ParentID
		needsSync = true
	}

	if local.StartTime != nil && server.StartTime == nil {
		diff.StartTime = local.StartTime
		needsSync = true
	} else if local.StartTime == nil && server.StartTime != nil {
		diff.StartTime = nil // TODO: this should never happen?
		needsSync = true
	} else if local.StartTime != nil && server.StartTime != nil && local.StartTime.UnixNano() != server.StartTime.UnixNano() {
		diff.StartTime = local.StartTime
		needsSync = true
	}

	if local.FinishTime != nil && server.FinishTime == nil {
		diff.FinishTime = local.FinishTime
		needsSync = true
	} else if local.FinishTime == nil && server.FinishTime != nil {
		diff.FinishTime = nil // TODO: this should never happen?
		needsSync = true
	} else if local.FinishTime != nil && server.FinishTime != nil && local.FinishTime.UnixNano() != server.FinishTime.UnixNano() {
		diff.FinishTime = local.FinishTime
		needsSync = true
	}

	if local.Result != server.Result {
		diff.Result = local.Result
		needsSync = true
	}

	if local.Log != server.Log {
		diff.Log = local.Log
		needsSync = true
	}

	if !needsSync {
		return RecordDiff{}, false
	}

	return diff, true
}

func convertToAPIDiffs(diffs []RecordDiff, runnerName string) []ghactions.TimelineRecordDiff {
	apiDiffs := make([]ghactions.TimelineRecordDiff, 0, len(diffs))

	for _, diff := range diffs {
		var log *ghactions.Log
		if diff.Log != nil {
			log = &ghactions.Log{
				ID: diff.Log.ID,
			}
		}
		tlDiff := ghactions.TimelineRecordDiff{
			ID:               string(diff.ID),
			ParentID:         (*string)(diff.ParentID),
			Type:             (*string)(diff.Type),
			Name:             diff.Name,
			StartTime:        diff.StartTime,
			FinishTime:       diff.FinishTime,
			CurrentOperation: nil,
			PercentComplete:  diff.PercentComplete,
			State:            (*string)(diff.State),
			Result:           (*string)(diff.Result),
			ResultCode:       nil,
			ChangeID:         &diff.ChangeID,
			LastModified:     time.Time{},
			WorkerName:       &runnerName,
			Order:            &diff.Order,
			RefName:          &diff.RefName,
			Log:              log,
			Details:          nil,
			ErrorCount:       0,
			WarningCount:     0,
			NoticeCount:      0,
			Issues:           []string{},
			Variables:        map[string]string{},
			Location:         diff.Location,
			PreviousAttempts: []int{},
			Attempt:          1,
			Identifier:       diff.Identifier,
		}

		apiDiffs = append(apiDiffs, tlDiff)
	}

	return apiDiffs
}
