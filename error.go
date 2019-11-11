package y

func RecoverError() {
	if err := recover(); err != nil {
		Error(err)
	}
}
