package crash

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"k8s.io/apimachinery/pkg/util/runtime"
)

func LogPanicAsJSON() {
	if r := recover(); r != nil {
		logPanicAsJSON(os.Stderr, r, time.Now(), debug.Stack())
		panic(r)
	}
}

func LogErrorAsJSON(err error) {
	logErrorAsJSON(os.Stderr, err, time.Now())
}

func logPanicAsJSON(out io.Writer, i interface{}, now time.Time, stack []byte) {
	bytes, err := json.Marshal(struct {
		Level   string `json:"level"`
		Time    string `json:"time"`
		Message string `json:"msg"`
		Stack   string `json:"stack"`
	}{
		Level:   "fatal",
		Time:    now.Format(time.RFC3339),
		Message: fmt.Sprintf("%v", i),
		Stack:   string(stack),
	})
	if err != nil {
		fmt.Fprintf(out, "error while serializing cmd exit panic: %+v\n", err) // nolint: errcheck
		fmt.Fprintf(out, "original panic: %+v\n", i)                           // nolint: errcheck
		return
	}

	fmt.Fprintf(out, "%s\n", bytes) // nolint: errcheck
}

func logErrorAsJSON(out io.Writer, inputErr error, now time.Time) {
	bytes, err := json.Marshal(struct {
		Level      string `json:"level"`
		Time       string `json:"time"`
		Message    string `json:"msg"`
		RawMessage string `json:"rawmsg"`
	}{
		Level:      "error",
		Time:       now.Format(time.RFC3339),
		Message:    inputErr.Error(),
		RawMessage: fmt.Sprintf("%+v", inputErr),
	})
	if err != nil {
		fmt.Fprintf(out, "error while serializing cmd exit err: %+v\n", err) // nolint: errcheck
		fmt.Fprintf(out, "original error: %+v\n", inputErr)                  // nolint: errcheck
		return
	}

	fmt.Fprintf(out, "%s\n", bytes) // nolint: errcheck
}

// this should be the first thing a cmd that uses apimachinery code invokes. It's a brittle hack to
// overwrite apimachinery's built in log handlers to use something more that uses json logging in
// more situations.
// This is an unfortunate trade off
func InstallAPIMachineryLoggers() {
	// overwrite apimachinery built in logging entirely (it just calls glog.errorf passing in the panic
	runtime.PanicHandlers = []func(interface{}){func(panic interface{}) {
		logPanicAsJSON(os.Stderr, panic, time.Now(), debug.Stack())
	}}

	// this override is more dangerous. It will fail if apimachinery change the ordering of the
	// built in error handler, the second entry should look like this:
	//  (&rudimentaryErrorBackoff{
	//		lastErrorTime: time.Now(),
	//		// 1ms was the number folks were able to stomach as a global rate limit.
	//		// If you need to log errors more than 1000 times a second you
	//		// should probably consider fixing your code instead. :)
	//		minPeriod: time.Millisecond,
	//	}).OnError,
	// and the OnError method should look like this:
	//  func (r *rudimentaryErrorBackoff) OnError(error) {
	//		r.lastErrorTimeLock.Lock()
	//		defer r.lastErrorTimeLock.Unlock()
	//		d := time.Since(r.lastErrorTime)
	//		if d < r.minPeriod {
	//			// If the time moves backwards for any reason, do nothing
	//			time.Sleep(r.minPeriod - d)
	//		}
	//		r.lastErrorTime = time.Now()
	//	}
	// similar issue in https://github.com/kubernetes/client-go/issues/18 around the mess that is glog
	rudimentaryErrorBackoff := runtime.ErrorHandlers[1]
	runtime.ErrorHandlers = []func(error){
		func(err error) {
			logErrorAsJSON(os.Stderr, err, time.Now())
		},
		rudimentaryErrorBackoff,
	}
}
