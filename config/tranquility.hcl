job "tranquility" {
	task "fix" {
		image = "pulcy/tranquility:latest"
		volumes = "/var/run/fleet.sock:/var/run/fleet.sock"
		type = "oneshot"
		timer = "hourly"
		args = [
			"fix",
		]
	}
}
