$UCRT = "C:\msys64\ucrt64"

$DIST = ".\dist"

New-Item -ItemType Directory -Force $DIST | Out-Null

# Copy your executable
Copy-Item ".\gateway.exe" $DIST -Force

# FreeRDP runtime
Copy-Item "$UCRT\bin\libfreerdp3.dll" $DIST -Force
Copy-Item "$UCRT\bin\libfreerdp-client3.dll" $DIST -Force
Copy-Item "$UCRT\bin\libwinpr3.dll" $DIST -Force

# Copy all dependent DLLs (recommended)
Copy-Item "$UCRT\bin\libgcc_s_seh-1.dll" $DIST -Force -ErrorAction SilentlyContinue
Copy-Item "$UCRT\bin\libstdc++-6.dll" $DIST -Force -ErrorAction SilentlyContinue
Copy-Item "$UCRT\bin\libwinpthread-1.dll" $DIST -Force -ErrorAction SilentlyContinue
Copy-Item "$UCRT\bin\libcrypto-3-x64.dll" $DIST -Force -ErrorAction SilentlyContinue
Copy-Item "$UCRT\bin\libssl-3-x64.dll" $DIST -Force -ErrorAction SilentlyContinue
Copy-Item "$UCRT\bin\zlib1.dll" $DIST -Force -ErrorAction SilentlyContinue