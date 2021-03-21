package validate

import "github.com/go-playground/validator/v10"

// FieldError holds the validation error information of a validated struct field.
//  It's the result of the validate.Struct function and is directly returned as error response in web handlers.
type FieldError struct {
	// Field is the name of the field.
	Field string `json:"field"`
	// Tag is the validation tag rule which triggered the error.
	Tag string `json:"tag"`
	// Value is the value which triggered the error.
	Value string `json:"value"`
	// Msg is the validation error message.
	Msg string `json:"msg"`
}

type CustomValidator struct {
	Tag            string
	Func           validator.Func
	CallEvenIfNull bool
}

// use a single instance of Validate, it caches struct info
var validate *validator.Validate

// Struct validates structs and returns FieldError for each invalid struct field.
// Struct fields must have validation tags to trigger validation.
// See https://pkg.go.dev/github.com/go-playground/validator/v10?#section-documentation for available validation rules and usage of them.
func Struct(s interface{}, customValidators ...CustomValidator) []FieldError {
	var errors []FieldError

	validate = validator.New()

	for _, customValidator := range customValidators {
		err := validate.RegisterValidation(customValidator.Tag, customValidator.Func, customValidator.CallEvenIfNull)
		if err != nil {
			panic(err)
		}
	}

	err := validate.Struct(s)

	if err != nil {
		// validator returns a InvalidValidationError if data is not validatable, e.g.: not a struct ...
		// in this case we also return a FieldError only containing the error msg
		if e, ok := err.(*validator.InvalidValidationError); ok {
			errors = append(errors, FieldError{
				Field: "",
				Tag:   "",
				Value: "",
				Msg:   e.Error(),
			})
			return errors
		}

		for _, err := range err.(validator.ValidationErrors) {
			errors = append(errors, FieldError{
				Field: err.StructNamespace(),
				Tag:   err.Tag(),
				Value: err.Param(),
				Msg:   err.Error(),
			})
		}
	}
	return errors
}

// IsStructValid structs and returns true if no validation error occurs.
// Struct fields must have validation tags to trigger validation.
// See https://pkg.go.dev/github.com/go-playground/validator/v10?#section-documentation for available validation rules and usage of them.
func IsStructValid(s interface{}, customValidators ...CustomValidator) bool {
	validate = validator.New()
	for _, customValidator := range customValidators {
		err := validate.RegisterValidation(customValidator.Tag, customValidator.Func, customValidator.CallEvenIfNull)
		if err != nil {
			panic(err)
			return false
		}
	}
	err := validate.Struct(s)
	return err == nil
}
