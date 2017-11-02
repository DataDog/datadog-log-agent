
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
  system("go build -tags=docker -o build/logagent ./pkg/logagent") || exit(1)
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
