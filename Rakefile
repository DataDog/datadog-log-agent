
def os
  case RUBY_PLATFORM
  when /linux/
    "linux"
  when /darwin/
    "darwin"
  when /x64-mingw32/
    "windows"
  else
    fail 'Unsupported OS'
  end
end

  
desc "Run go fmt"
task :fmt do
  `go fmt ./pkg/...`
end

desc "Run go lint"
task :lint do
  `golint ./pkg/...`
end

desc "Run go vet"
task :vet do
  `go vet ./pkg/...`
end

desc "Run go race"
task :test_race => %w[fmt lint vet] do
  system("go build -tags=docker -race -o build/logagent ./pkg/logagent") || exit(1)
  system("go test -tags=docker ./pkg/...") || exit(1)
end

desc "Run go test and linters"
task :test => %w[fmt lint vet] do
    system("go test -tags=docker ./pkg/...") || exit(1)
end

desc "Build the agent"
task :build => %w[fmt lint vet] do
  case os
  when "windows"
    bin = "logagent.exe"
  else 
    bin = "logagent"
  end
  if ENV['windres'] then
    # first compile the message table, as it's an input to the resource file
    msgcmd = "windmc --target pe-x86-64 -r pkg/logagent/windows_resources pkg/logagent/windows_resources/log-agent-msg.mc"
    puts msgcmd
    sh msgcmd
    # for now, hardcode the version
    agentversion = "1.0.0"
    ver_array = agentversion.split(".")
    rescmd = "windres --define MAJ_VER=#{ver_array[0]} --define MIN_VER=#{ver_array[1]} --define PATCH_VER=#{ver_array[2]} "
    rescmd += "-i pkg/logagent/windows_resources/log-agent.rc --target=pe-x86-64 -O coff -o pkg/logagent/rsrc.syso"
    sh rescmd

  end
  
  system("go build -tags=docker -o build/#{bin} ./pkg/logagent") || exit(1)
end

desc "Build the agent on linux amd64"
task :build_linux_amd64 do
  puts("building for linux amd64")
  system("env GOOS=linux GOARCH=amd64 go build -tags=docker -o build/linux-amd64 ./pkg/logagent") || exit(1)
end


desc "Build the agent on windows amd64"
task :build_windows_amd64 do
  puts("building for windows amd64")
  puts("Not supported")
  # system("env GOOS=windows GOARCH=amd64 go build -tags=docker -o build/windows-amd64 ./pkg/logagent") || exit(1)
end

desc "Build the agent on different platforms"
task :build_all => %w[fmt lint vet build_linux_amd64 build_windows_amd64] do
  puts("Done building on all platforms")
end

desc "Install the agent"
task :install do
    system("go install -tags=docker ./pkg/logagent") || exit(1)
end

desc "Setup Go dependencies"
task :deps do
  system("go get github.com/Masterminds/glide")
  system("go get -u github.com/golang/lint/golint")
  system("glide install")
end
