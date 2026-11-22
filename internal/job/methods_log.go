package job

import (
	"os"
)

func (writer *logWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.size+int64(len(data)) > logLimit {
		if err := writer.file.Close(); err != nil {
			return 0, err
		}

		if err := os.Remove(writer.path + ".1"); err != nil && !os.IsNotExist(err) {
			return 0, err
		}

		if err := os.Rename(writer.path, writer.path+".1"); err != nil {
			return 0, err
		}

		file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
		if err != nil {
			return 0, err
		}

		writer.file = file
		writer.size = 0
		writer.rotated = true
	}

	count, err := writer.file.Write(data)
	writer.size += int64(count)
	writer.total += int64(count)
	return count, err
}

func (writer *logWriter) progress() (int64, bool) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.total, writer.rotated
}
