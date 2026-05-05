package kumopolicy

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// Lint parses the rendered Lua source with a sandboxed gopher-lua VM. Any
// syntax error is returned as a string suitable for surfacing to the UI.
//
// It does NOT execute the policy — kumomta's runtime API is not available
// here. Lint covers syntactic validity only, which catches the most common
// rendering bugs and any escape-violation that produced invalid Lua.
func Lint(src string) (issues []string) {
	L := lua.NewState(lua.Options{
		SkipOpenLibs:        true,
		IncludeGoStackTrace: false,
	})
	defer L.Close()
	if _, err := L.LoadString(src); err != nil {
		return []string{fmt.Sprintf("lua syntax: %v", err)}
	}
	return nil
}
