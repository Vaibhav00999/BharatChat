package validator

import (
	"github.com/go-playground/validator/v10"
	"regexp"
)

var e164Regex = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

func New() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	_ = v.RegisterValidation("e164", func(fl validator.FieldLevel) bool { return e164Regex.MatchString(fl.Field().String()) })
	return v
}
