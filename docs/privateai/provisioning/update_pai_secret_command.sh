#!/bin/bash

kubectl patch secret paisecret --patch-file secretupdate.yaml -n pai
