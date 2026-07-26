package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestEveryModelParses(t *testing.T) {
	for _, name := range modelNames() {
		got, err := parseModel(name)
		if err != nil || string(got) != name {
			t.Errorf("%q gave %q %v", name, got, err)
		}
	}

	if _, err := parseModel("blob"); err == nil {
		t.Error("an unknown model was accepted")
	}
	if got, err := parseModel(""); err != nil || got == "" {
		t.Errorf("empty name should give a default, got %q %v", got, err)
	}
}

// The geometry builders run without a GL context up to the point of upload, so
// their maths can be checked here. Anything that would upload is left to the
// live run.
func TestPrismRejectsTooFewSides(t *testing.T) {
	if _, err := buildTaperedPrism(mgl32.Vec3{1, 1, 1}, 1, 1, 1, 2); err == nil {
		t.Error("a two sided prism was accepted")
	}
}

// Every model has to be listed and have a real height. A zero one would be
// invisible, and a missing builder would only show up when somebody selected it.
func TestEveryModelIsBuildable(t *testing.T) {
	for _, kind := range modelKinds {
		t.Run(string(kind), func(t *testing.T) {
			spec, ok := models[kind]
			if !ok {
				t.Fatal("listed but has no spec")
			}
			if spec.build == nil {
				t.Error("no builder")
			}
			if spec.height <= 0 {
				t.Errorf("height %v, want a shape with some height", spec.height)
			}
		})
	}

	if len(models) != len(modelKinds) {
		t.Errorf("%d specs against %d listed kinds", len(models), len(modelKinds))
	}
}

// The pylon is the tallest on purpose. Silhouette is the only thing that
// survives the conversion to block characters, so if the heights ever get
// shuffled the reason for having it disappears.
func TestPylonIsTheTallest(t *testing.T) {
	pylon := models[modelPylon].height
	for kind, spec := range models {
		if kind != modelPylon && spec.height >= pylon {
			t.Errorf("%s is %v against the pylon's %v", kind, spec.height, pylon)
		}
	}
}
