package controller

import (
	"github.com/nautilus/cluster-operator/pkg/controller/nautilusupgrade"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, nautilusupgrade.Add)
}
