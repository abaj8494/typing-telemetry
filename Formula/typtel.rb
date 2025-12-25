class Typtel < Formula
  desc "Keystroke telemetry for developers - track your daily typing"
  homepage "https://github.com/aayushbajaj/typing-telemetry"
  version "0.1.0"
  license "MIT"

  # For local development, use: brew install --build-from-source ./Formula/typtel.rb
  url "file://#{HOMEBREW_PREFIX}/src/typing-telemetry", using: :git, branch: "main"

  depends_on :macos
  depends_on "go" => :build

  def install
    system "go", "mod", "download"

    # Build CLI
    system "go", "build", *std_go_args(ldflags: "-s -w"), "-o", bin/"typtel", "./cmd/typtel"

    # Build menu bar app
    ENV["CGO_ENABLED"] = "1"
    system "go", "build", *std_go_args(ldflags: "-s -w"), "-o", bin/"typtel-menubar", "./cmd/typtel-menubar"

    # Build daemon
    system "go", "build", *std_go_args(ldflags: "-s -w"), "-o", bin/"typtel-daemon", "./cmd/daemon"
  end

  def post_install
    # Create LaunchAgent
    (prefix/"com.typtel.menubar.plist").write <<~EOS
      <?xml version="1.0" encoding="UTF-8"?>
      <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
      <plist version="1.0">
      <dict>
          <key>Label</key>
          <string>com.typtel.menubar</string>
          <key>ProgramArguments</key>
          <array>
              <string>#{opt_bin}/typtel-menubar</string>
          </array>
          <key>RunAtLoad</key>
          <true/>
          <key>KeepAlive</key>
          <dict>
              <key>SuccessfulExit</key>
              <false/>
          </dict>
          <key>ProcessType</key>
          <string>Interactive</string>
      </dict>
      </plist>
    EOS
  end

  def caveats
    <<~EOS
      To start typtel menu bar on login:
        mkdir -p ~/Library/LaunchAgents
        cp #{opt_prefix}/com.typtel.menubar.plist ~/Library/LaunchAgents/
        launchctl load ~/Library/LaunchAgents/com.typtel.menubar.plist

      IMPORTANT: You must grant Accessibility permissions:
        1. Open System Preferences > Privacy & Security > Accessibility
        2. Click the lock to make changes
        3. Add #{opt_bin}/typtel-menubar to the list
        4. Restart the menu bar app

      To view your stats: typtel
      To view today's count: typtel today
    EOS
  end

  test do
    system "#{bin}/typtel", "today"
  end
end
