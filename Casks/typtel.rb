cask "typtel" do
  version "0.9.0"
  sha256 :no_check  # Updated automatically after first release

  url "https://github.com/abaj8494/typing-telemetry/releases/download/v#{version}/Typtel-#{version}.zip"
  name "Typtel"
  desc "Keystroke and mouse telemetry for developers"
  homepage "https://github.com/abaj8494/typing-telemetry"

  # Install the app to /Applications
  app "Typtel.app"

  # Symlink CLI to /usr/local/bin
  binary "Typtel.app/Contents/MacOS/typtel"

  # Uninstall: stop service and remove LaunchAgent
  uninstall launchctl: "com.typtel.menubar"

  zap trash: [
    "~/.local/share/typtel",
    "~/Library/LaunchAgents/com.typtel.menubar.plist",
  ]

  caveats <<~EOS
    Typtel requires Accessibility permissions to track keystrokes.

    SETUP:
      1. Open System Settings > Privacy & Security > Accessibility
      2. Click + and select /Applications/Typtel.app
      3. Enable the checkbox

    START:
      Open Typtel from Spotlight (Cmd+Space, type "Typtel")
      Or run: open /Applications/Typtel.app

    The app will appear in your menu bar.

    COMMANDS:
      typtel           - Interactive dashboard
      typtel stats     - Show statistics
      typtel today     - Today's keystroke count
      typtel test      - Typing speed test

    To start automatically at login, enable "Launch at Login" in the menu bar.
  EOS
end
