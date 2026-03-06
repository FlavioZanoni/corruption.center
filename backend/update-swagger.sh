#!/bin/bash

# need the swagger tool: go install github.com/swaggo/swag/cmd/swag@latest
swag init -g api/main.go -o api/docs
