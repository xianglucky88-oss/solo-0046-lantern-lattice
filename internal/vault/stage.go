package vault

import "os"

func stage(dir string, data []byte) (name string, err error) {
	f, err := os.CreateTemp(dir, ".staging-")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(f.Name())
		}
	}()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return f.Name(), err
}
