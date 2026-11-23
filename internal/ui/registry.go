package ui

import "fmt"

// Views register themselves: each screen_<name>.go calls registerView from its
// init(), so adding a view never edits the hub. A menu entry whose name has
// no constructor renders as unavailable.

// viewCtor builds a view from the services.
type viewCtor func(Services) screen

var viewCtors = map[string]viewCtor{}

// registerView adds a view constructor under name. Registering a name twice is a
// programming error and panics at start-up.
func registerView(name string, ctor viewCtor) {
	if ctor == nil {
		panic(fmt.Sprintf("ui: nil constructor for view %q", name))
	}
	if _, dup := viewCtors[name]; dup {
		panic(fmt.Sprintf("ui: view %q registered twice", name))
	}
	viewCtors[name] = ctor
}

// viewFor returns the constructor for name, and false when no view is
// registered under it.
func viewFor(name string) (viewCtor, bool) {
	ctor, ok := viewCtors[name]
	return ctor, ok
}
