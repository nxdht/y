package y

import "github.com/fsnotify/fsnotify"

type FileWatcher struct {
	*fsnotify.Watcher
}

func NewFileWatcher(f func(fileName string)) (fileWatcher *FileWatcher, err error) {
	var watcher *fsnotify.Watcher
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					Error("FileWatcher Events closed")
					return
				}

				if event.Op&fsnotify.Write == fsnotify.Write {
					f(event.Name)
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					Error("FileWatcher Errors closed")
					return
				}

				Error("FileWatcher error: ", err)
			}
		}
	}()

	return &FileWatcher{
		Watcher: watcher,
	}, nil
}
