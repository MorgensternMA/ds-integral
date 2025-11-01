#!/bin/bash
echo "Building Integral Master..."
go build -o master
if [ $? -eq 0 ]; then
    echo "Master built successfully!"
    echo "Executable: master"
else
    echo "Build failed!"
    exit 1
fi
