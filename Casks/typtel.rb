cask "typtel" do
  version "1.0.0"
  sha256 "9361af7997ddc7d3f272b57d8aabb93e771cdf71dc8db54172d8f89c34348da5"

  url "https://github.com/abaj8494/homebrew-typing-telemetry/releases/download/v#{version}/Typtel-#{version}.zip"
  name "Typtel"
  desc "Keystroke and mouse distance metrics for developers"
  homepage "https://github.com/abaj8494/typing-telemetry"

  # Install the app to /Applications
  app "Typtel.app"

  # Remove quarantine to prevent "app is damaged" error (unsigned app)
  postflight do
    system_command "/usr/bin/xattr",
                   args: ["-cr", "#{appdir}/Typtel.app"],
                   sudo: false
  end

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
