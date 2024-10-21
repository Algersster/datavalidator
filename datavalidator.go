package datavalidator

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

var (
	ErrNotStruct = errors.New("wrong argument given, should be a struct")
	ErrEmptyValidator = errors.New("validator tag is empty")
	ErrInvalidValidatorSyntax = errors.New("invalid validator syntax")
	ErrInvalidValidatorType = errors.New("invalid validator type")
	ErrValidateForUnexportedFields = errors.New("validation for unexported field is not allowed")
	ErrUnsupportedType = errors.New("field type is unsupported")
	ErrUnsupportedValidatorType = errors.New("validator type is unsupported")
)
const validateTag string = "validate"

type ValidationError struct {
	Name	string
	Err		error
}
func (e *ValidationError) Error() string {
	return fmt.Sprintf("Field %s, error: %s", e.Name, e.Err.Error())
}
func (e *ValidationError) Is(trg error) bool {
	return e.Err == trg
}

type ValidationErrors []ValidationError
func (errs *ValidationErrors) AddCustomError(errors ...ValidationError) {
	*errs = append(*errs, errors...)
}
func (errs *ValidationErrors) AddError(name string, errors ...error) {
	for _, e := range errors {
		errs.AddCustomError(ValidationError{ name, e })
	}
}
func (errs ValidationErrors) Error() string {
	es := ""
	for i, err := range errs {
		if i > 0 {
			es += "\n"
		}
		es += err.Error()
	}

	return es
}
func (errs ValidationErrors) Is(trg error) bool {
	for _, err := range errs {
		if err.Err == trg {
			return true
		}
	}
	return false
}

type checkType struct {
	Type	string
	Value	any
}
type checkTypes []checkType

type fieldCheck struct {
	Name		string
	RefValue	reflect.Value
	RefStruct	reflect.StructField
	CheckTypes	*checkTypes
	Validator	*structValidator
}

type structValidator struct {
	RefValue	reflect.Value
	ParentName	string
	Errors		*ValidationErrors
}

type Validator interface {
	Execute() 	*ValidationErrors
}

func (ct *checkType) Init(t reflect.Kind) error {
	var (
		val any
		err error
	)
	sval := ct.Value.(string)
	
	switch ct.Type {
	case "len":
		if t != reflect.String {
			return ErrUnsupportedValidatorType
		}
		val, err = strconv.Atoi(sval)
	case "min", "max":
		val, err = strconv.Atoi(sval)
	case "in":
		instrs := strings.Split(sval, ",")
		if sval == "" || len(instrs) == 0 {
			return ErrInvalidValidatorSyntax
		}
		if t == reflect.String {
			val = instrs
		} else {
			inints := make([]int, len(instrs))
			for i, v := range instrs {
				inints[i], err = strconv.Atoi(v)
				if err != nil {
					return ErrInvalidValidatorSyntax
				}
			}
			val = inints
		}
	default:
		return ErrInvalidValidatorType
	}
	if err != nil {
		return ErrInvalidValidatorSyntax
	}
	ct.Value = val

	return nil
}

func (c *fieldCheck) Check(v any, t any) []error {
	var errs []error
	switch t {
	case reflect.Int:
		errs = c.CheckInt(v.(int))
	case reflect.String:
		errs = c.CheckString(v.(string))
	}
	return errs
}
func (c *fieldCheck) ParseTag(t string, vt reflect.Kind) error {
	c.CheckTypes = &checkTypes{}
	if t == "" {
		return ErrEmptyValidator
	}
	
	tags := strings.Split(t, ";")
	for _, tag := range tags {
		tp := strings.Split(tag, ":")
		if len(tp) < 2 || tp[0] == "" || tp[1] == "" {
			return ErrInvalidValidatorSyntax
		}
		ct := checkType{tp[0],tp[1]}
		if err := ct.Init(vt); err != nil {
			return err
		}
		*c.CheckTypes = append(*c.CheckTypes, ct)
	}

	return nil
}
func (c *fieldCheck) CheckString(s string) []error {
	errs := []error{}
	for _, ct := range *c.CheckTypes {
		var err error
		switch ct.Type {
		case "len":
			if ok := ValidateLength(s, ct.Value.(int)); !ok {
				err = fmt.Errorf("string length is not match - '%d'", ct.Value.(int))
			}
		case "min":
			if ok := ValidateLengthRangeMin(s, ct.Value.(int)); !ok {
				err = fmt.Errorf("string length is less than '%d'", ct.Value.(int))
			}
		case "max":
			if ok := ValidateLengthRangeMax(s, ct.Value.(int)); !ok {
				err = fmt.Errorf("string length is bigger than '%d'", ct.Value.(int))
			}
		case "in":
			ins := ct.Value.([]string)
			if ok := ValidateIn(s, ins); !ok {
				err = fmt.Errorf("string value is not contains in %s", ins)
			}
		}
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}
func (c *fieldCheck) CheckInt(i int) []error {
	errs := []error{}
	for _, ct := range *c.CheckTypes {
		var err error
		switch ct.Type {
		case "min":
			ci := ct.Value.(int)
			if ok := ValidateRange(i, ci, i); !ok {
				err = fmt.Errorf("value %d is less than %d", i, ci)
			}
		case "max":
			ci := ct.Value.(int)
			if ok := ValidateRange(i, i, ci); !ok {
				err = fmt.Errorf("value %d is bigger than %d", i, ci)
			}
		case "in":
			ins := ct.Value.([]int)
			if ok := ValidateIn(i, ins); !ok {
				err = fmt.Errorf("value %d is not contains in %d", i, ins)
			}
		}
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}
func (c *fieldCheck) EvalCheck() {
	kt := c.RefStruct.Type.Kind()
	if kt == reflect.Struct {
		innerv := InitValidator(c.RefValue, c.Name)
		errs := innerv.Execute()
		if errs != nil {
			c.Validator.Errors.AddCustomError(*errs...)
		}
		return
	}

	vtag, ok := c.RefStruct.Tag.Lookup(validateTag)
	if !ok {
		return
	}
	if !c.RefStruct.IsExported() {
		c.Validator.Errors.AddCustomError(ValidationError{ c.Name, ErrValidateForUnexportedFields })
		return
	}

	if kt == reflect.Array || kt == reflect.Slice {
		if c.RefValue.Len() == 0 {
			return
		}
		kt = c.RefValue.Index(0).Type().Kind()
	}

	switch kt {
	case reflect.Int, reflect.String:
		if err := c.ParseTag(vtag, kt); err != nil {
			c.Validator.Errors.AddCustomError(ValidationError{ c.Name, err })
			return
		}
	default:
		c.Validator.Errors.AddCustomError(ValidationError{ c.Name, ErrUnsupportedType })
		return
	}

	switch c.RefStruct.Type.Kind() {
	case reflect.Array, reflect.Slice:
		for i := 0; i < c.RefValue.Len(); i++ {
			errs := c.Check(c.RefValue.Index(i).Interface(), kt)
			if len(errs) > 0 {
				name := fmt.Sprintf("%s[%d]", c.Name, i)
				c.Validator.Errors.AddError(name, errs...)
			}
		}
	case reflect.Int, reflect.String:
		errs := c.Check(c.RefValue.Interface(), kt)
		if len(errs) > 0 {
			c.Validator.Errors.AddError(c.Name, errs...)
		}
	}
}

func (vld *structValidator) NewFieldCheck(s reflect.StructField, v reflect.Value) *fieldCheck {
	name := ""
	if (vld.ParentName != "") {
		name = vld.ParentName + "."
	}
	name += s.Name
	return &fieldCheck{ Name: name, RefStruct: s, RefValue: v, Validator: vld }
}

func (vld *structValidator) Execute() *ValidationErrors {
	RefType := vld.RefValue.Type()
	if RefType.Kind() != reflect.Struct {
		vld.Errors.AddCustomError(ValidationError{ "Main", ErrNotStruct })
		return vld.Errors
	}

	for i:=0; i < RefType.NumField(); i++ {
		fCheck := vld.NewFieldCheck(RefType.Field(i), vld.RefValue.Field(i))
		fCheck.EvalCheck()
	}

	if len(*vld.Errors) > 0 {
		return vld.Errors
	}
	
	return nil
}

func InitValidator(v any, s string) Validator {
	vld := &structValidator{ ParentName: s, Errors: &ValidationErrors{} }
	if rval, ok := v.(reflect.Value); ok {
		vld.RefValue = rval
	} else {
		vld.RefValue = reflect.ValueOf(v)
	}

	return vld
}

func ValidateLength(v string, c int) bool {
	len := len([]rune(v))
	return len == c
}
func ValidateLengthRangeMin(v string, cmin int) bool {
	len := len([]rune(v))
	return ValidateRange(len, cmin, len)
}
func ValidateLengthRangeMax(v string, cmax int) bool {
	len := len([]rune(v))
	return ValidateRange(len, len, cmax)
}
func ValidateRange(v int, cmin int, cmax int) bool {
	return v >= cmin && v <= cmax
}
func ValidateIn[T comparable](v T, ins []T) bool {
	for _, in := range ins {
		if v == in {
			return true
		}
	}
	return false
}

func Validate (av any) error {
	validator := InitValidator(av, "")
	errs := validator.Execute()
	if (errs != nil) {
		return *errs
	}
	return nil
}