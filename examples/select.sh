#!/bin/bash

echo "go-l2p2 exampels";
echo "----------------";
echo "";

PS3="Choose an example to try: "

while :
do
  options=("data-channel" "webtransport")
  select lng in "${options[@]}"
  do
      case $lng in
          "data-channel")
              echo "";
              echo "Running data-channel example";
              echo "See https://github.com/backkem/go-lp2p/tree/main/examples/data-channel for details";
              echo "";
              go run ./examples/data-channel;
              echo "";;
          "webtransport")
              echo "";
              echo "Running webtransport example";
              echo "See https://github.com/backkem/go-lp2p/tree/main/examples/webtransport-pooled";
              echo "";
              go run ./examples/webtransport-pooled;
              echo "";;
          *)
             echo "Ooops";;
      esac
  done
 echo ""
 echo ""
done