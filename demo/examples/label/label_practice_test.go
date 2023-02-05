package label

import (
	"fmt"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"testing"
)

func TestLabelUse(t *testing.T) {

	mylabels := labels.Set{
		"app": "aaa",
		"bbb": "bbb",
	}

	sel := labels.NewSelector()
	req, err := labels.NewRequirement("bbb", selection.Equals, []string{"bbb"})
	if err != nil {
		panic(err.Error())
	}
	sel.Add(*req)
	if sel.Matches(mylabels) {
		fmt.Printf("Selector %v matched field set %v\n", sel, mylabels)
	} else {
		panic("Selector should have matched field set")
	}

	// Selector from string expression.
	sel, err = labels.Parse("app==aaa")
	if err != nil {
		panic(err.Error())
	}
	if sel.Matches(mylabels) {
		fmt.Printf("Selector %v matched label set %v\n", sel, mylabels)
	} else {
		panic("Selector should have matched labels")
	}

}
