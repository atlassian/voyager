load("@io_bazel_rules_docker//go:image.bzl", "go_image")
load("@io_bazel_rules_go//go:def.bzl", "go_binary")

def define_command_targets(name):
    go_binary(
        name = name,
        embed = [":go_default_library"],
        pure = "on",
        visibility = ["//visibility:public"],
    )

    go_binary(
        name = name + "_race",
        embed = [":go_default_library"],
        race = "on",
        tags = ["manual"],
        visibility = ["//visibility:public"],
    )

    go_image(
        name = "container",
        binary = ":" + name,
        tags = ["manual"],
        visibility = ["//visibility:public"],
    )

    # Use CC base image here to have glibc installed because it is
    # needed for race detector to work https://github.com/golang/go/issues/14481
    # Otherwise getting:
    # error while loading shared libraries: libstdc++.so.6: cannot open shared object file: No such file or directory
    go_image(
        name = "container_race",
        base = "@cc_debug_image_base//image",
        binary = ":" + name + "_race",
        tags = ["manual"],
        visibility = ["//visibility:public"],
    )
