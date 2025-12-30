class TypingTelemetry < Formula
  desc "Keystroke telemetry for developers - track your daily typing"
  homepage "https://github.com/abaj8494/typing-telemetry"
  version "0.4.0"
  license "MIT"

  # Install from GitHub repository
  url "https://github.com/abaj8494/typing-telemetry.git", tag: "v0.4.0"
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
  end

  # Use Homebrew's service block for LaunchAgent management
  service do
    run opt_bin/"typtel-menubar"
    keep_alive true
    process_type :interactive
    log_path var/"log/typtel-menubar.log"
    error_log_path var/"log/typtel-menubar.log"
    environment_variables HOME: Dir.home
  end

  def caveats
    <<~EOS
      To start the typing telemetry menu bar app:
        brew services start typing-telemetry

      To stop:
        brew services stop typing-telemetry

      IMPORTANT: You must grant Accessibility permissions:
        1. Open System Settings > Privacy & Security > Accessibility
        2. Click the lock to make changes
        3. Add #{opt_bin}/typtel-menubar to the list
        4. Restart with: brew services restart typing-telemetry

      Available commands:
        typtel           - Open interactive dashboard
        typtel stats     - Show typing statistics
        typtel today     - Show today's keystroke count
        typtel test      - Start a typing speed test

      The menu bar app (typtel-menubar) includes the keystroke daemon.
      Alternatively, you can run typtel-daemon directly for headless operation.
    EOS
  end

  test do
    # Basic test - create storage directory and test CLI
    system "#{bin}/typtel", "today"
  end
end
