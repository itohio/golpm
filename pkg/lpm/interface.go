package lpm

// Device defines the interface for LPM devices (real or mocked).
type Device interface {
	Connect() error
	Close() error
	Samples() <-chan RawSample
	SetHeaters(heater1, heater2, heater3 bool) error
	IsConnected() bool
}

// Ensure Device implements DeviceInterface.
var _ Device = (*Serial)(nil)

// Ensure MockedDevice implements DeviceInterface.
var _ Device = (*Mock)(nil)
