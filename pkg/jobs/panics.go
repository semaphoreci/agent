package jobs

import "fmt"

func PreventPanicPropagation(callback func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("[panic] Prevented panic propagation.", r)
		}
	}()

	callback()
}
