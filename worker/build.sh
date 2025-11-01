#!/bin/bash
echo "Building Integral Worker..."
go build -o worker
if [ $? -eq 0 ]; then
    echo "Worker built successfully!"
    echo "Executable: worker"
else
    echo "Build failed!"
    exit 1
fi
