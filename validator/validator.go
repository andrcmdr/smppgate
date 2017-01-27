package validator

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
)

func ValidateValueByTag(value string, tagString string) error {
	tags := strings.Split(tagString, " ")
	for _, tag := range tags {
		if tag != "" {
			keyvalue := strings.Split(tag, "=")
			if len(keyvalue) > 1 {
				if keyvalue[0] == "regexp" {
					r, err := regexp.Compile(keyvalue[1])
					if err != nil {
						return err
					}
					if !r.MatchString(value) {
						return errors.New("Value '" + value + "' not matched regular expression " + keyvalue[1])
					}
				}
			}
		}
	}
	return nil
}

func validateR(rv reflect.Value, tagValue string, fieldName string) error {
	if rv.Type().Name() == "Time" {
		return nil
	}
	switch rv.Kind() {
	case reflect.Struct:
		for i := 0; i < rv.NumField(); i++ {
			tagValue := rv.Type().Field(i).Tag.Get("validate")
			fieldName := rv.Type().Field(i).Name
			if err := validateR(rv.Field(i), tagValue, fieldName); err != nil {
				return err
			}
		}
	case reflect.Slice:
		for j := 0; j < rv.Len(); j++ {
			if err := validateR(rv.Index(j), "", ""); err != nil {
				return err
			}
		}
	case reflect.Ptr:
		if rv.IsNil() {
			return nil
		}
		if err := validateR(rv.Elem(), "", ""); err != nil {
			return err
		}
	case reflect.String:
		if tagValue != "" {
			value := rv.String()
			if err := ValidateValueByTag(value, tagValue); err != nil {
				return errors.New("Field '" + fieldName + "': " + err.Error())
			}
		}
	}
	return nil
}

func Validate(v interface{}) error {
	rv := reflect.ValueOf(v)
	return validateR(rv, "", "")
}
