package forecastpipe

// Fork supports atomic A3-A6 transactions without changing earlier stage logic.
func (p *Pipeline) Fork() *Pipeline {
	return &Pipeline{config: CloneConfig(p.config), analysis: p.analysis.Fork()}
}
