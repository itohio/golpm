//go:generate tinygo flash -target=xiao

package main

import "fmt"

func main() {
	PIN_HEATER1.Configure(machine.PinConfig{Mode: machine.PinOutput})
	PIN_HEATER2.Configure(machine.PinConfig{Mode: machine.PinOutput})
	PIN_HEATER3.Configure(machine.PinConfig{Mode: machine.PinOutput})
	PIN_ADC.Configure(machine.PinConfig{Mode: machine.PinInput})
	PIN_VOLTAGE_ADC.Configure(machine.PinConfig{Mode: machine.PinInput})

	for {
		reading := PIN_ADC.Get()
		voltage := PIN_VOLTAGE_ADC.Get()

		print(fmt.Sprintf("%d\n", voltage))
	}
}
