#!/bin/sh

version=$(cat VERSION)
themes=$(cat THEMES)
IFS=$'\n'


for theme in $themes; do
    echo "Building theme: $theme"
    cd $theme
    npm install
    # Check if args contains --lint
    if [[ $* == *--lint* ]]; then
        echo "Linting..."
        npm run lint
    fi
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$version npm run build
    cd ..
done
