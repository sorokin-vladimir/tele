package keys

// Resolve returns the Action for the given key in ctx, falling back to ContextGlobal.
func (km KeyMap) Resolve(ctx Context, key string) Action {
	if ctxMap, ok := km[ctx]; ok {
		if action, ok := ctxMap[key]; ok {
			return action
		}
	}
	if global, ok := km[ContextGlobal]; ok {
		if action, ok := global[key]; ok {
			return action
		}
	}
	return ActionNone
}
