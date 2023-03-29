package kubernetes

import (
	"fmt"
	"regexp"

	"github.com/semaphoreci/agent/pkg/api"
)

type ImageValidator struct {
	Expressions []regexp.Regexp
}

func NewImageValidator(expressions []string) (*ImageValidator, error) {
	regexes := []regexp.Regexp{}
	for _, exp := range expressions {
		r, err := regexp.Compile(exp)
		if err != nil {
			return nil, err
		}

		regexes = append(regexes, *r)
	}

	return &ImageValidator{Expressions: regexes}, nil
}

func (v *ImageValidator) Validate(containers []api.Container) error {
	for _, container := range containers {
		if err := v.validateImage(container.Image); err != nil {
			return err
		}
	}

	return nil
}

func (v *ImageValidator) validateImage(image string) error {
	for _, expression := range v.Expressions {
		if expression.MatchString(image) {
			return nil
		}
	}

	return fmt.Errorf("image '%s' is not allowed", image)
}
