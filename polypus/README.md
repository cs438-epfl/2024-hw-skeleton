<div align="center">
    <img width="200px" src="https://github.com/cs438-epfl/assets/blob/main/polypus/logo small.png">
</div>

<p></p>

# Debugging tool for your distributed system

## Introduction

Polypus is a research tool developed at DEDIS. If offers a visualization and
analysis tool for distributed systems. As part of CS438, you might want to use
it for debugging and getting insights about your implementation.

Polypus a web-based application that connects to your nodes. To use Polypus you
must go to [https://polypus.dedis.ch](https://polypus.dedis.ch) and provide the
configuration of your system. In this package we provide a simple ready-to-use
script that runs some nodes and gives you the Polypus configuration. 

The script allows you to test a **broadcast** of message and **tag** (to check
your Paxos implementation), among a set of N nodes.

## Quick setup

1️⃣ From this folder, run a set of nodes with `NUM_PEERS=4 go run .`:

<p></p>

<div align="center">
    <img src="https://github.com/cs438-epfl/assets/blob/main/polypus/start.png">
</div>

<p></p>

⚠️ Make sure to disable any sort of logging in your nodes, e.g. by setting `GLOG` environment variable to `no` (as per the requirements in HW0).

2️⃣ You can copy the Polypus configuration, open
[Polypus](https://polypus.dedis.ch), enter the configuration with the gear icon
at the top right, and start visualizing your system:

<p></p>

<div align="center">
    <img src="https://github.com/cs438-epfl/assets/blob/main/polypus/config.gif">
</div>

<p></p>

3️⃣ You can then play around by broadcasting messages and use the tag
functionality. Note that you can changes the number of nodes, and additionally
provide a `LOG_FILE=log.json` variable when you launch the script, which will
save the logging activity into a file.

## Polypus features

### Replay

Polypus offers live and replay functionalities. By default it shows the live
activity, but you can stop the live with the cross icon at the bottom, and then
start dragging through history with the timeline. You can play and set the speed
with the clock icon on the far right.

<p></p>

<div align="center">
    <img src="https://github.com/cs438-epfl/assets/blob/main/polypus/replay.gif">
</div>

<p></p>

### Check the content of messages

Click on any message to see its content. We display the full content of messages:

<p></p>

<div align="center">
    <img src="https://github.com/cs438-epfl/assets/blob/main/polypus/see content.gif">
</div>

<p></p>

### Expand, hide, focus

Use the buttons on the top right to configure the view. You can resize, hide the
activity of nodes, and focus on a single node.

<p></p>

<div align="center">
    <img src="https://github.com/cs438-epfl/assets/blob/main/polypus/view params.gif">
</div>

<p></p>

## Support

Polypus is still in development and you might find some bugs or unexpected
behavior. In our mission to help you as much as possible we welcome any
feedback.

For support, remarks, or anything concerning Polypus do not hesitate to contact the TA team on Moodle.
