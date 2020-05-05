package aggregate

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/TIBCOSoftware/flogo-contrib/activity/aggregate/window"
	"github.com/TIBCOSoftware/flogo-lib/core/activity"
	"github.com/TIBCOSoftware/flogo-lib/logger"
	"github.com/magallardo/flogo-contrib/activity/aggregate/support"
)

// activityLogger is the default logger for the Aggregate Activity
var activityLogger = logger.GetLogger("activity-aggregate")

const (
	sFunction           = "function"
	sWindowType         = "windowType"
	sWindowSize         = "windowSize"
	sResolution         = "resolution"
	sProceedOnlyOnEmit  = "proceedOnlyOnEmit"
	sAdditionalSettings = "additionalSettings"

	ivValue = "value"

	ovResult = "result"
	ovReport = "report"

	sdWindow = "window"
)

//we can generate json from this! - we could also create a "validate-able" object from this
type Settings struct {
	Function           string `md:"function,required,allowed(avg,sum,min,max,count)"`
	WindowType         string `md:"windowType,required,allowed(tumbling,sliding,timeTumbling,timeSliding)"`
	WindowSize         int    `md:"windowSize,required"`
	ProceedOnlyOnEmit  bool
	Resolution         int
	AdditionalSettings map[string]string
}

func init() {
	activityLogger.SetLogLevel(logger.InfoLevel)
}

func New(config *activity.Config) (activity.Activity, error) {
	act := &AggregateActivity{mutex: &sync.RWMutex{}}

	//todo implement
	//config.Settings

	return act, nil
}

// AggregateActivity is an Activity that is used to Aggregate a message to the console
type AggregateActivity struct {
	metadata *activity.Metadata
	mutex    *sync.RWMutex
}

// NewActivity creates a new AppActivity
func NewActivity(md *activity.Metadata) activity.Activity {
	return &AggregateActivity{mutex: &sync.RWMutex{}, metadata: metadata}
}

// Metadata returns the activity's metadata
func (a *AggregateActivity) Metadata() *activity.Metadata {
	return a.metadata
}

// Eval implements api.Activity.Eval - Aggregates the Message
func (a *AggregateActivity) Eval(ctx activity.Context) (done bool, err error) {

	//todo move to Activity instance creation
	settings, err := getSettings(ctx)
	if err != nil {
		return false, err
	}

	ss, ok := activity.GetSharedTempDataSupport(ctx)
	if !ok {
		return false, fmt.Errorf("AggregateActivity not supported by this activity host")
	}

	sharedData := ss.GetSharedTempData()
	wv, defined := sharedData[sdWindow]

	timerSupport, timerSupported := support.GetTimerSupport(ctx)

	var w window.Window

	//create the window & associated timer if necessary

	if !defined {

		a.mutex.Lock()

		wv, defined = sharedData[sdWindow]
		if defined {
			w = wv.(window.Window)
		} else {
			w, err = createWindow(ctx, settings)

			if err != nil {
				a.mutex.Unlock()
				return false, err
			}

			sharedData[sdWindow] = w
		}

		a.mutex.Unlock()
	} else {
		w = wv.(window.Window)
	}

	in := ctx.GetInput(ivValue)

	emit, result := w.AddSample(in)

	if timerSupported {
		timerSupport.UpdateTimer(true)
	}

	ctx.SetOutput(ovResult, result)
	ctx.SetOutput(ovReport, emit)

	done = !(settings.ProceedOnlyOnEmit && !emit)

	return done, nil
}

func createWindow(ctx activity.Context, settings *Settings) (w window.Window, err error) {

	timerSupport, timerSupported := support.GetTimerSupport(ctx)

	windowSettings := &window.Settings{Size: settings.WindowSize, ExternalTimer: timerSupported, Resolution: settings.Resolution}
	windowSettings.SetAdditionalSettings(settings.AdditionalSettings)

	wType := strings.ToLower(settings.WindowType)

	switch wType {
	case "tumbling":
		w, err = NewTumblingWindow(settings.Function, windowSettings)
	case "sliding":
		w, err = NewSlidingWindow(settings.Function, windowSettings)
	case "timetumbling":
		w, err = NewTumblingTimeWindow(settings.Function, windowSettings)
		if timerSupported {
			timerSupport.CreateTimer(time.Duration(settings.WindowSize)*time.Millisecond, moveWindow, true)
		}
	case "timesliding":
		w, err = NewSlidingTimeWindow(settings.Function, windowSettings)
		if timerSupported {
			timerSupport.CreateTimer(time.Duration(settings.Resolution)*time.Millisecond, moveWindow, true)
		}
	default:
		return nil, fmt.Errorf("unsupported window type: '%s'", settings.WindowType)
	}

	return w, err
}

func (a *AggregateActivity) PostEval(ctx activity.Context, userData interface{}) (done bool, err error) {
	return true, nil
}

func moveWindow(ctx activity.Context) bool {

	ss, _ := activity.GetSharedTempDataSupport(ctx)
	sharedData := ss.GetSharedTempData()

	wv, _ := sharedData[sdWindow]

	w, _ := wv.(window.TimeWindow)

	emit, result := w.NextBlock()

	ctx.SetOutput(ovResult, result)
	ctx.SetOutput(ovReport, emit)

	poe := ctx.GetInput(sProceedOnlyOnEmit).(bool)

	return !(poe && !emit)
}

func getSettings(ctx activity.Context) (*Settings, error) {

	settings := &Settings{}

	settings.Function = ctx.GetInput(sFunction).(string)
	settings.WindowType = ctx.GetInput(sWindowType).(string)
	settings.WindowSize = ctx.GetInput(sWindowSize).(int)
	settings.Resolution = ctx.GetInput(sResolution).(int)
	settings.ProceedOnlyOnEmit = ctx.GetInput(sProceedOnlyOnEmit).(bool)
	settings.AdditionalSettings, _ = toParams(ctx.GetInput(sAdditionalSettings).(string))

	// settings validation can be done here once activities are created on configuration instead of
	// setting up during runtime

	return settings, nil
}

func toParams(values string) (map[string]string, error) {

	if values == "" {
		return map[string]string{}, nil
	}

	var params map[string]string

	result := strings.Split(values, ",")
	params = make(map[string]string)
	for _, pair := range result {
		nv := strings.Split(pair, "=")
		if len(nv) != 2 {
			return nil, fmt.Errorf("invalid settings")
		}
		params[nv[0]] = nv[1]
	}

	return params, nil
}
