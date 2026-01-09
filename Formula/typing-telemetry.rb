class TypingTelemetry < Formula
  desc "Keystroke and mouse telemetry for developers - track your daily typing and mouse movement"
  homepage "https://github.com/abaj8494/typing-telemetry"
  version "1.1.7"
  license "MIT"

  # Install from GitHub repository
  url "https://github.com/abaj8494/typing-telemetry.git", tag: "v1.1.7"
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

    # Create .app bundle for accessibility permissions
    app_contents = prefix/"Typtel.app/Contents"
    app_contents.mkpath
    (app_contents/"MacOS").mkpath
    (app_contents/"Resources").mkpath

    # Copy binary into app bundle (symlinks don't work well with macOS permissions)
    cp bin/"typtel-menubar", app_contents/"MacOS/typtel-menubar"

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
  # Run from /Applications/Typtel.app after user copies it there
  service do
    run ["/Applications/Typtel.app/Contents/MacOS/typtel-menubar"]
    keep_alive true
    process_type :interactive
    log_path var/"log/typtel-menubar.log"
    error_log_path var/"log/typtel-menubar.log"
    environment_variables HOME: Dir.home
  end

  def caveats
    <<~EOS
      RECOMMENDED: Use the cask instead for a proper /Applications install:
        brew uninstall typing-telemetry
        brew install --cask typtel

      If using the formula, setup requires:
        1. Copy app: cp -R #{opt_prefix}/Typtel.app /Applications/
        2. Grant accessibility to /Applications/Typtel.app
        3. Start: brew services start typing-telemetry

      COMMANDS:
        typtel           - Interactive dashboard
        typtel stats     - Show statistics
        typtel today     - Today's keystroke count
        typtel test      - Typing speed test
        typtel v         - View charts in browser
    EOS
  end

  test do
    system "#{bin}/typtel", "today"
  end
end
