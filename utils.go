package y

import (
	"fmt"
	"github.com/gofrs/uuid"
	"time"
)

func Ternary(cond bool, trueValue, falseValue interface{}) interface{} {
	if cond {
		return trueValue
	} else {
		return falseValue
	}
}

func GoForever(f func() bool) {
	go func() {
		for {
			if func() bool {
				defer RecoverError()

				return f()
			}() {
				return
			}
		}
	}()
}

var uuidNamespace = uuid.Must(uuid.FromString("9117b810-9dad-23d1-80b4-00c04fd43011"))

func GenUUID5(name string) string {
	name = fmt.Sprintf("%s%d", name, time.Now().UnixNano())
	u := uuid.NewV5(uuidNamespace, name)
	return fmt.Sprintf("%s", u)
}
