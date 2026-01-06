# EPIC 01: Basic GUI Application with Configuration

## Overview

Create the foundational Fyne application structure with configuration management, basic window layout, and toolbar buttons.

## Goals

- Set up Fyne application framework
- Implement configuration package that reads from YAML file
- Create main window with toolbar containing Connect, Settings, and Measure buttons
- Provide default configuration values
- Command-line flag for serial port override (-p)

## Requirements

### Configuration Package (`config`)

**Structure:**
- Read configuration from YAML file (default: `config.yaml` in working directory)
- Provide default values when file doesn't exist or fields are missing
- Expose configuration struct with all necessary fields

**Configuration Fields:**
```yaml
serial:
  port: "COM3"  # Serial port (Windows) or "/dev/ttyACM0" (Linux/Mac)

voltage_divider:
  r1: 20000     # Resistor R1 in ohms (to positive)
  r2: 20000     # Resistor R2 in ohms (to ground)
  vref: 3.3     # ADC reference voltage

heaters:
  - resistance: 2300  # Heater 1 resistance in ohms
  - resistance: 500   # Heater 2 resistance in ohms
  - resistance: 200   # Heater 3 resistance in ohms

measurement:
  window_seconds: 10    # Measurement window size in seconds (Phase 1)
  pulse_threshold: 0.001 # Threshold for pulse detection (V/s) (Phase 1)

calibration:
  # Calibration process parameters (Phase 3)
  baseline_duration: 10s      # Baseline measurement duration
  heater_duration: 2s         # Heater measurement duration
  cooloff_duration: 20s        # Cooloff wait duration between heaters
  heater_sequence: [1, 2, 3]  # Heater order (by index, 1-based)
  
  # Calibration points (populated after calibration process)
  points:
    - slope: 0.0      # V/s
      power: 0.0       # mW
    # Additional points added during calibration
```

**Package Interface:**
- `Load(filename string) (*Config, error)` - Load config from file, use defaults if missing
- `Save(filename string) error` - Save config to file
- `Default() *Config` - Return default configuration
- `Config` struct - Data structure only, no calculation methods

**Configuration Structure Notes:**
- Duration fields (`baseline_duration`, `heater_duration`, `cooloff_duration`) should be parsed as Go `time.Duration` (e.g., "10s", "2s", "20s")
- `heater_sequence` is an array of heater indices (1-based: 1, 2, 3)
- `pulse_threshold` is in V/s (volts per second)
- `calibration.points` array is populated during calibration process (Phase 3)
- All numeric values should use appropriate types (float32 for voltages/powers, int for indices)

**Design Principles:**
- Single Responsibility: Only handles configuration data storage and loading/saving
- Zero values useful: Default() returns usable configuration
- Explicit dependencies: Filename passed as parameter
- No package-level state: Each operation accepts Config struct
- **NO CALCULATIONS**: Config package only stores and provides access to configuration parameters. Calculations (such as heater power) belong in other packages (e.g., converter package)

### Main Application Structure

**Package:** `main` or `app`

**Components:**
1. **Application Window**
   - Title: "Laser Power Meter"
   - Default size: 1200x800
   - Layout: Border layout with toolbar at top

2. **Toolbar**
   - **Connect Button**: Will connect to serial port (initially disabled/placeholder)
   - **Settings Button**: Opens settings dialog (initially placeholder)
   - **Measure Button**: Starts/stops measurement (initially disabled)

3. **Main Content Area**
   - Initially empty/placeholder
   - Will be used for graph widget in later epics
   - Should be designed for oscilloscope-style display with slow horizontal axis
   - Horizontal timebase: 5-10 seconds per full window width (configurable via measurement window)

**Command Line Flags:**
- `-p, -port`: Override serial port from configuration
- `-config`: Specify config file path (default: `config.yaml`)

## Implementation Steps

1. **Create `config` package**
   - Define Config struct with all fields (data only, no methods)
   - Implement Default() function
   - Implement Load() function using `gopkg.in/yaml.v3`
   - Implement Save() function
   - **NO calculation methods** - Config package only provides data access

2. **Create main application structure**
   - Set up Fyne app and window
   - Create toolbar with three buttons
   - Wire up command-line flags
   - Load configuration (use flag for port override)
   - Store config in application state

3. **Add error handling**
   - Handle missing config file gracefully (use defaults)
   - Display error dialogs for critical failures
   - Log errors appropriately

## Dependencies

- `fyne.io/fyne/v2` - GUI framework
- `gopkg.in/yaml.v3` - YAML parsing
- `flag` - Command-line flag parsing

## Testing Considerations

- Unit tests for config package:
  - Default configuration values
  - Loading valid YAML
  - Loading invalid YAML (should use defaults)
  - Saving configuration
  - Field access (verify data is correctly loaded)

## Success Criteria

- ✅ Application starts and displays window with toolbar
- ✅ Configuration loads from YAML file or uses defaults
- ✅ Command-line flag overrides serial port
- ✅ All buttons visible (functionality comes in later epics)
- ✅ Configuration can be saved
- ✅ Code follows SOLID principles
- ✅ Zero values are useful (default config works)

## Notes

- Keep configuration struct simple and flat where possible
- Consider using options pattern for config loading if needed
- **Config package is data-only**: No calculations belong here
- Heater power calculations will be in converter package (EPIC 04)
- The GUI should resemble an oscilloscope with very slow horizontal sweep
- Horizontal timebase typically 5-10 seconds per full window (as configured)
- This slow sweep allows visualization of thermal response over time

**Configuration Evolution:**
- Phase 1: Basic measurement fields (`window_seconds`, `pulse_threshold`)
- Phase 3: Calibration fields added (`baseline_duration`, `heater_duration`, `cooloff_duration`, `heater_sequence`, `points`)
- Configuration should be backward compatible (missing fields use defaults)
- Calibration points are written to config after calibration process completes

