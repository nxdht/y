package y

import "github.com/fsnotify/fsnotify"

type FileWatcher struct {
	*fsnotify.Watcher
}

func NewFileWatcher(f func(fileName string, mode string)) (fileWatcher *FileWatcher, err error) {
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
					f(event.Name, "write")
				} else if event.Op&fsnotify.Create == fsnotify.Create {
					f(event.Name, "create")
				} else if event.Op&fsnotify.Rename == fsnotify.Rename {
					f(event.Name, "rename")
				} else if event.Op&fsnotify.Remove == fsnotify.Remove {
					f(event.Name, "remove")
				} else if event.Op&fsnotify.Chmod == fsnotify.Chmod {
					f(event.Name, "chmod")
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
