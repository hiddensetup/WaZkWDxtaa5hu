go build -gcflags="all=-N -l" -o wa
dlv --listen=:2345 --headless=true --api-version=2 exec ./wa