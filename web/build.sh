#!/bin/bash

version=$(cat VERSION)
pwd

while IFS= read -r theme; do
    echo "Building theme: $theme"
    rm -r build/$theme
    cd "$theme"
    npm install
    # Check if args contains --lint
    if [[ $* == *--lint* ]]; then
        echo "Linting..."
        npm run lint
    fi
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$version npm run build
    cd ..
done < THEMES
