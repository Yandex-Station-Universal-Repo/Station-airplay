#define _GNU_SOURCE
#include <sched.h>
#include <sys/mount.h>
#include <unistd.h>
#include <stdio.h>
#include <limits.h>

/* Only this child sees the generated Avahi account. Never write /etc. */
int main(int argc, char **argv) {
    char path[PATH_MAX];
    if (argc < 3) return 2;
    if (unshare(CLONE_NEWNS) || mount(NULL, "/", NULL, MS_REC | MS_PRIVATE, NULL)) {
        perror("private mount namespace"); return 3;
    }
    const char *names[] = {"passwd", "group"};
    for (int i = 0; i < 2; i++) {
        char target[32];
        if (snprintf(path, sizeof(path), "%s/%s", argv[1], names[i]) >= (int)sizeof(path)) return 4;
        snprintf(target, sizeof(target), "/etc/%s", names[i]);
        if (mount(path, target, NULL, MS_BIND, NULL) ||
            mount(NULL, target, NULL, MS_BIND | MS_REMOUNT | MS_RDONLY, NULL)) {
            perror("private Avahi account"); return 4;
        }
    }
    execv(argv[2], argv + 2);
    perror("exec Avahi"); return 5;
}
