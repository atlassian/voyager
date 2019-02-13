"""
Macros for cmd.
"""

load("@io_bazel_rules_docker//go:image.bzl", "go_image")
load("@io_bazel_rules_go//go:def.bzl", "go_binary")

def define_command_targets(name, binary_embed):
    go_binary(
        name = name,
        embed = binary_embed,
        pure = "on",
        tags = ["manual"],
        visibility = ["//visibility:public"],
    )

    go_binary(
        name = name + "_race",
        embed = binary_embed,
        race = "on",
        visibility = ["//visibility:public"],
    )

    go_image(
        name = "container",
        base = "//cmd:nobody_image",
        binary = ":" + name,
        tags = ["manual"],
        visibility = ["//visibility:public"],
    )

    go_image(
        name = "container_race",
        base = "//cmd:nobody_image_debug",
        binary = ":" + name + "_race",
        tags = ["manual"],
        visibility = ["//visibility:public"],
    )
