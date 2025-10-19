package gui

import (
	"fmt"

	"github.com/jroimartin/gocui"
)

// open the application controls
// TODO: view size can handled better for such situations like too small
// application area
func (gui *Gui) openCheatSheetView(g *gocui.Gui, _ *gocui.View) error {
	maxX, maxY := g.Size()
	v, err := g.SetView(cheatSheetViewFeature.Name, maxX/2-30, maxY/2-12, maxX/2+30, maxY/2+12)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = cheatSheetViewFeature.Title
		v.Wrap = false

		fmt.Fprintln(v, " "+yellow.Sprint("Use ↑/↓ or mouse wheel to scroll"))
		fmt.Fprintln(v, "")

		// Organize keybindings by category for better readability
		fmt.Fprintln(v, " "+cyan.Sprint("Mode:"))
		gui.printKeyBinding(v, "f", "Fetch [default]")
		gui.printKeyBinding(v, "p", "Pull")

		fmt.Fprintln(v, "")
		fmt.Fprintln(v, " "+cyan.Sprint("Navigation:"))
		gui.printKeyBinding(v, "↑ / k", "Cursor Up")
		gui.printKeyBinding(v, "↓ / j", "Cursor Down")
		gui.printKeyBinding(v, "g", "Jump to Top")
		gui.printKeyBinding(v, "G", "Jump to Bottom")
		gui.printKeyBinding(v, "ctrl + b", "Page Up")
		gui.printKeyBinding(v, "ctrl + f", "Page Down")
		gui.printKeyBinding(v, "ctrl + u", "Page Up")
		gui.printKeyBinding(v, "page up", "Page Up")
		gui.printKeyBinding(v, "page down", "Page Down")
		gui.printKeyBinding(v, "home", "Home")
		gui.printKeyBinding(v, "end", "End")

		fmt.Fprintln(v, "")
		fmt.Fprintln(v, " "+cyan.Sprint("Actions:"))
		gui.printKeyBinding(v, "space", "Select")
		gui.printKeyBinding(v, "enter", "Start")
		gui.printKeyBinding(v, "a / ctrl + space", "Select All")
		gui.printKeyBinding(v, "backspace", "Deselect All")

		fmt.Fprintln(v, "")
		fmt.Fprintln(v, " "+cyan.Sprint("Views:"))
		gui.printKeyBinding(v, "b", "Branches")
		gui.printKeyBinding(v, "B", "Batch branch checkout")
		gui.printKeyBinding(v, "h", "Help")
		gui.printKeyBinding(v, "tab", "Back to Overview")
		gui.printKeyBinding(v, "→", "Next Panel")
		gui.printKeyBinding(v, "←", "Prev Panel")

		fmt.Fprintln(v, "")
		fmt.Fprintln(v, " "+cyan.Sprint("Sorting:"))
		gui.printKeyBinding(v, "n", "Sort by Name")
		gui.printKeyBinding(v, "d", "Sort by Date")

		fmt.Fprintln(v, "")
		fmt.Fprintln(v, " "+cyan.Sprint("Other:"))
		gui.printKeyBinding(v, "q", "Quit")
		gui.printKeyBinding(v, "esc", "Close View")
	}
	return gui.focusToView(cheatSheetViewFeature.Name)
}

// helper function to print a keybinding in consistent format
func (gui *Gui) printKeyBinding(v *gocui.View, key, description string) {
	binding := " " + cyan.Sprint(key) + ": " + description
	fmt.Fprintln(v, binding)
}

// close the application controls and do the clean job
func (gui *Gui) closeCheatSheetView(g *gocui.Gui, v *gocui.View) error {
	if err := g.DeleteView(v.Name()); err != nil {
		return nil
	}
	return gui.closeViewCleanup(mainViewFeature.Name)
}
