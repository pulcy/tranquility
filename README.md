# Tranquility

[Fleet](https://github.com/coreos/fleet) sometimes abandons units in a failed or inactive state.
Tranquility detects those units and tries to restart them.

This utility is intended to be run on a timer inside a fleet cluster.

## Usage

Check for invalid units:
```
docker run -it -v /var/run/fleet.sock:/var/run/fleet.sock pulcy/tranquility:latest check
```

Fix invalid units:
```
docker run -it -v /var/run/fleet.sock:/var/run/fleet.sock pulcy/tranquility:latest fix
```
