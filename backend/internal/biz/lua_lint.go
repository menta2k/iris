package biz

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// LintLua parses the rendered policy with a sandboxed gopher-lua VM and returns
// any syntax errors. It does NOT execute the policy (KumoMTA's runtime API is
// not available), so it covers syntactic validity only — which catches the most
// common rendering bugs and any escaping defect that produced invalid Lua.
//
// Adapted from the production Iris policy linter.
func LintLua(src string) []string {
	L := lua.NewState(lua.Options{SkipOpenLibs: true, IncludeGoStackTrace: false})
	defer L.Close()
	if _, err := L.LoadString(src); err != nil {
		return []string{fmt.Sprintf("lua syntax: %v", err)}
	}
	return nil
}
