package y

import (
	"bufio"
	"fmt"
	"github.com/cihub/seelog"
	"os"
	"runtime/debug"
	"strings"
	"sync"
)

var (
	mutex  sync.Mutex
	logger seelog.LoggerInterface
	Logger = LogFunc(Info)

	errNotInit = fmt.Errorf("log not init")
)

func InitLog(fileName string) error {
	if fileName == "" {
		return InitLogDefault()
	}
	var err error
	if strings.HasPrefix(fileName, "/") {
		logger, err = seelog.LoggerFromConfigAsFile(fileName)
	} else {
		logger, err = seelog.LoggerFromConfigAsFile(fmt.Sprintf("config/%s.xml", fileName))
	}
	if err != nil {
		return err
	}

	logger.SetAdditionalStackDepth(1)
	//seelog.ReplaceLogger(logger)

	return nil
}

func InitLogDefault() error {
	index := strings.LastIndex(os.Args[0], string(os.PathSeparator))
	if index == -1 {
		return fmt.Errorf("InitLogDefault can't find os.PathSeparator in path: %s", os.Args[0])
	}

	name := os.Args[0][index+1:]

	content := fmt.Sprintf(`
<seelog type="asyncloop">
	<outputs formatid="main">
		<rollingfile type="size" filename="%s" maxsize="1000000" maxrolls="100"/>
		<console />
	</outputs>
	<formats>
        <format id="main" format="[%%Date(2006-01-02 15:04:05.000)][%%Level][%%File:%%Line] %%Msg%%n"/>
    </formats>
</seelog>
`, fmt.Sprintf("log%s%s_all.log", string(os.PathSeparator), name))

	var err error
	logger, err = seelog.LoggerFromConfigAsString(content)
	if err != nil {
		return err
	}

	logger.SetAdditionalStackDepth(1)

	return nil
}

func FlushLog() {
	logger.Flush()
}

// Debugf formats message according to format specifier
// and writes to default logger with ylog level = Debug.
func Debugf(format string, params ...interface{}) {
	if logger == nil {
		return
	}
	mutex.Lock()
	defer mutex.Unlock()
	logger.Debugf(format, params...)
}

// Infof formats message according to format specifier
// and writes to default logger with ylog level = Info.
func Infof(format string, params ...interface{}) {
	if logger == nil {
		return
	}
	mutex.Lock()
	defer mutex.Unlock()
	logger.Infof(format, params...)
}

// Errorf formats message according to format specifier and writes to default logger with ylog level = Error
func Errorf(format string, params ...interface{}) error {
	if logger == nil {
		return errNotInit
	}
	mutex.Lock()
	defer mutex.Unlock()
	return logger.Errorf(format, params...)
}

// Debug formats message using the default formats for its operands and writes to default logger with ylog level = Debug
func Debug(v ...interface{}) {
	if logger == nil {
		return
	}
	mutex.Lock()
	defer mutex.Unlock()

	var t []interface{}
	for _, u := range v {
		t = append(t, u, " ")
	}
	logger.Debug(t...)
}

// Info formats message using the default formats for its operands and writes to default logger with ylog level = Info
func Info(v ...interface{}) {
	if logger == nil {
		return
	}
	mutex.Lock()
	defer mutex.Unlock()

	var t []interface{}
	for _, u := range v {
		t = append(t, u, " ")
	}
	logger.Info(t...)
}

// Error formats message using the default formats for its operands and writes to default logger with ylog level = Error
func Error(v ...interface{}) error {
	if logger == nil {
		return errNotInit
	}
	mutex.Lock()
	defer mutex.Unlock()

	var t []interface{}
	for _, u := range v {
		t = append(t, u, " ")
	}
	return logger.Error(t...)
}

func Panic(err interface{}) {
	if logger == nil {
		return
	}
	if err != nil {
		panic(printPanic(err))
	}
}

func printPanic(err interface{}) error {
	if logger == nil {
		return errNotInit
	}
	mutex.Lock()
	defer mutex.Unlock()

	logger.Error("==============================================================")
	err1 := logger.Error(err)
	buf := debug.Stack()
	for {
		advance, data, _ := bufio.ScanLines(buf, true)
		if data == nil {
			break
		}
		logger.Error(string(data))
		buf = buf[advance:]
	}
	logger.Error("--------------------------------------------------------------")
	return err1
}

type LogFunc func(keyvals ...interface{})

func (l LogFunc) Log(keyvals ...interface{}) error {
	l(keyvals...)
	return nil
}
