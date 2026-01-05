class TypingTelemetry < Formula
  desc "Keystroke and mouse telemetry for developers - track your daily typing and mouse movement"
  homepage "https://github.com/abaj8494/typing-telemetry"
  version "0.8.5"
  license "MIT"

  # Install from GitHub repository
  url "https://github.com/abaj8494/typing-telemetry.git", tag: "v0.8.5"
  head "https://github.com/abaj8494/typing-telemetry.git", branch: "main"

  depends_on :macos
  depends_on "go" => :build

  def install
    system "go", "mod", "download"

    ldflags = "-s -w -X main.Version=#{version}"

    # Build CLI (no CGO required)
    system "go", "build", *std_go_args(ldflags: ldflags), "-o", bin/"typtel", "./cmd/typtel"

    # Build menu bar app (requires CGO for macOS frameworks)
    ENV["CGO_ENABLED"] = "1"
    system "go", "build", *std_go_args(ldflags: ldflags), "-o", bin/"typtel-menubar", "./cmd/typtel-menubar"

    # Build daemon (requires CGO for macOS frameworks)
    system "go", "build", *std_go_args(ldflags: ldflags), "-o", bin/"typtel-daemon", "./cmd/daemon"

    # Create .app bundle for accessibility permissions
    app_contents = prefix/"Typtel.app/Contents"
    app_contents.mkpath
    (app_contents/"MacOS").mkpath
    (app_contents/"Resources").mkpath

    # Symlink binary into app bundle (shares permissions with bin/)
    (app_contents/"MacOS/typtel-menubar").make_symlink(bin/"typtel-menubar")

    # Copy icon if it exists
    icon_path = buildpath/"assets/AppIcon.icns"
    cp icon_path, app_contents/"Resources/AppIcon.icns" if icon_path.exist?

    # Create Info.plist with icon reference
    (app_contents/"Info.plist").write <<~XML
      <?xml version="1.0" encoding="UTF-8"?>
      <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
      <plist version="1.0">
      <dict>
          <key>CFBundleExecutable</key>
          <string>typtel-menubar</string>
          <key>CFBundleIdentifier</key>
          <string>com.typtel.menubar</string>
          <key>CFBundleName</key>
          <string>Typtel</string>
          <key>CFBundleDisplayName</key>
          <string>Typtel</string>
          <key>CFBundlePackageType</key>
          <string>APPL</string>
          <key>CFBundleVersion</key>
          <string>#{version}</string>
          <key>CFBundleShortVersionString</key>
          <string>#{version}</string>
          <key>CFBundleIconFile</key>
          <string>AppIcon</string>
          <key>LSMinimumSystemVersion</key>
          <string>10.13</string>
          <key>LSUIElement</key>
          <true/>
          <key>NSHighResolutionCapable</key>
          <true/>
          <key>NSHumanReadableCopyright</key>
          <string>Copyright 2024 Aayush Bajaj. MIT License.</string>
      </dict>
      </plist>
    XML
  end

  # Use Homebrew's service block for LaunchAgent management
  # Run from app bundle (symlinks to bin/, so permissions are shared)
  service do
    run [opt_prefix/"Typtel.app/Contents/MacOS/typtel-menubar"]
    keep_alive true
    process_type :interactive
    log_path var/"log/typtel-menubar.log"
    error_log_path var/"log/typtel-menubar.log"
    environment_variables HOME: Dir.home
  end

  def post_install
    # Ensure ~/Applications exists
    user_apps = Pathname.new(Dir.home)/"Applications"
    user_apps.mkpath

    # Force remove and recreate symlink using shell commands
    # (Ruby's unlink fails due to macOS quarantine attributes on ~/Applications)
    target = user_apps/"Typtel.app"
    system "rm", "-rf", target
    system "ln", "-sf", opt_prefix/"Typtel.app", target

    # Touch the app bundle to update Finder/Spotlight
    system "touch", opt_prefix/"Typtel.app"
  end

  def caveats
    <<~EOS
      Typtel v#{version} installed!

      FIRST TIME SETUP:
        1. Open System Settings > Privacy & Security > Accessibility
        2. Click + and press Cmd+Shift+G
        3. Paste: ~/Applications/Typtel.app
        4. Enable the checkbox for Typtel
        5. Start: brew services start typing-telemetry

      You can also launch Typtel from Spotlight (Cmd+Space, type "Typtel")
      to restart the menubar if it disappears.

      COMMANDS:
        typtel           - Interactive dashboard
        typtel stats     - Show statistics
        typtel today     - Today's keystroke count
        typtel test      - Typing speed test

      SERVICE:
        brew services start typing-telemetry
        brew services stop typing-telemetry
        brew services restart typing-telemetry
    EOS
  end

  test do
    system "#{bin}/typtel", "today"
  end
end
