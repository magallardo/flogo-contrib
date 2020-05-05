package aggregate

import (
	"fmt"
	"strings"
	"sync"

	"github.com/TIBCOSoftware/flogo-contrib/activity/aggregate"
	"github.com/TIBCOSoftware/flogo-contrib/activity/aggregate/window"
	"github.com/TIBCOSoftware/flogo-lib/core/activity"
	"github.com/TIBCOSoftware/flogo-lib/logger"
	"github.com/magallardo/flogo-contrib/activity/aggregate/support"
)

const (
	ivFunction           = "function"
	ivWindowType         = "windowType"
	ivWindowSize         = "windowSize"
	ivResolution         = "resolution"
	ivProceedOnlyOnEmit  = "proceedOnlyOnEmit"
	ivAdditionalSettings = "additionalSettings"

	ivValue = "value"

	ovResult = "result"
	ovReport = "report"

	sdWindow = "window"
)

var activityLog = logger.GetLogger("tibco-activity-aggregate")

type AggregateActivity struct {
	metadata *activity.Metadata
	mutex    sync.Mutex
}

func NewActivity(metadata *activity.Metadata) activity.Activity {
	return &AggregateActivity{metadata: metadata}
}

func (a *AggregateActivity) Metadata() *activity.Metadata {
	return a.metadata
}

func (a *AggregateActivity) Eval(context activity.Context) (done bool, err error) {
	activityLog.Info("Executing Aggregate activity")

	sharedDataSupport, _ := activity.GetSharedTempDataSupport(context)
	sharedData := sharedDataSupport.GetSharedTempData()
	wv, defined := sharedData[sdWindow]

	timerSupport, timerSupported := support.GetTimerSupport(context)

	var w window.Window

	//create the window & associated timer if necessary

	if !defined {

		a.mutex.Lock()

		wv, defined = sharedData[sdWindow]
		if defined {
			w = wv.(window.Window)
		} else {
			w, err = a.createWindow(context)

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

	//Read Inputs
	if context.GetInput(ivValue) == nil {
		// Value is not configured
		// return error to the engine
		return false, activity.NewError("Value is not configured", "AGGREGATE-4001", nil)
	}
	in := context.GetInput(ivValue)

	emit, result := w.AddSample(in)

	if timerSupported {
		timerSupport.UpdateTimer(true)
	}

	err = context.SetOutput(ovResult, result)

	if err != nil {
		return false, err
	}

	err = context.SetOutput(ovReport, emit)
	if err != nil {
		return false, err
	}

	proceedOnlyOnEmit := context.GetInput(ivProceedOnlyOnEmit).(bool)
	done = !(proceedOnlyOnEmit && !emit)

	return done, nil
}

func (a *AggregateActivity) createWindow(context activity.Context) (w window.Window, err error) {

	function := context.GetInput(ivFunction).(string)
	windowType := context.GetInput(ivWindowType).(string)
	windowSize := context.GetInput(ivWindowSize).(int)
	resolution := context.GetInput(ivResolution).(int)
	proceedOnlyOnEmit := context.GetInput(ivProceedOnlyOnEmit).(bool)
	additionalSettingsParams, _ := toParams(context.GetInput(ivAdditionalSettings).(string))
	additionalSettings := additionalSettingsParams

	timerSupport, timerSupported := support.GetTimerSupport(context)

	windowSettings := &window.Settings{Size: windowSize, ExternalTimer: timerSupported, Resolution: resolution}
	err = windowSettings.SetAdditionalSettings(additionalSettings)
	if err != nil {
		return nil, err
	}

	wType := strings.ToLower(windowType)

	switch wType {
	case "tumbling":
		w, err = aggregate.NewTumblingWindow(function, windowSettings)
	case "sliding":
		w, err = aggregate.NewSlidingWindow(function, windowSettings)
	case "timetumbling":
		w, err = aggregate.NewTumblingTimeWindow(function, windowSettings)
		// if err == nil && timerSupported {
		// 	err = timerSupport.CreateTimer(time.Duration(windowSize)*time.Millisecond, a.moveWindow, true)
		// }
	case "timesliding":
		w, err = aggregate.NewSlidingTimeWindow(function, windowSettings)
		// if err == nil && timerSupported {
		// 	err = timerSupport.CreateTimer(time.Duration(resolution)*time.Millisecond, a.moveWindow, true)
		// }
	default:
		return nil, fmt.Errorf("unsupported window type: '%s'", windowType)
	}

	return w, err
}

func (a *AggregateActivity) moveWindow(context activity.Context) bool {

	proceedOnlyOnEmit := context.GetInput(ivProceedOnlyOnEmit).(bool)
	sharedData := GetSharedTempDataSupport(context)

	wv, _ := sharedData[sdWindow]

	w, _ := wv.(window.TimeWindow)

	emit, result := w.NextBlock()

	err := context.SetOutput(ovResult, result)
	if err != nil {
		//todo log error?
	}

	err = context.SetOutput(ovReport, emit)
	if err != nil {
		//todo log error?
	}

	return !(proceedOnlyOnEmit && !emit)
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
