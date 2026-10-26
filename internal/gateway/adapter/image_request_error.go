package adapter

// ImageRequestError identifies a caller field without including its contents
// or any private transport configuration in the public error.
type ImageRequestError struct {
	Param   string
	Message string
}

func (err *ImageRequestError) Error() string { return err.Message }

func (err *ImageRequestError) Unwrap() error { return ErrInvalidImageRequest }

func invalidImageField(param, message string) error {
	return &ImageRequestError{Param: param, Message: message}
}
