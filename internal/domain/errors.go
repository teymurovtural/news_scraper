package domain

import "errors"

var (
	ErrSourceNotFound  = errors.New("mənbə tapılmadı")
	ErrItemNotFound    = errors.New("xəbər tapılmadı")
	ErrDuplicateItem   = errors.New("bu xəbər artıq mövcuddur")
	ErrDuplicateSource = errors.New("bu mənbə artıq mövcuddur")
)
