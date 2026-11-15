/* runrss: run a command, print "<wall_us> <max_rss_kb> <exit_code>" on fd 3 (or stdout when fd 3 is closed).
 * A tiny parent matters: ru_maxrss of a child also contains the size of the forked parent image
 * before exec, so measuring from a big Python process would report Python's RSS, not the tool's. */
#define _GNU_SOURCE
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/resource.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

int main(int argc, char **argv) {
  if (argc < 2) return 2;
  struct timespec a, b;
  clock_gettime(CLOCK_MONOTONIC, &a);
  pid_t p = fork();
  if (p == 0) {
    int dn = open("/dev/null", O_RDWR);
    dup2(dn, 0); dup2(dn, 1); dup2(dn, 2);
    execvp(argv[1], argv + 1);
    _exit(127);
  }
  int st; struct rusage ru;
  wait4(p, &st, 0, &ru);
  clock_gettime(CLOCK_MONOTONIC, &b);
  long us = (b.tv_sec - a.tv_sec) * 1000000L + (b.tv_nsec - a.tv_nsec) / 1000;
  printf("%ld %ld %d\n", us, ru.ru_maxrss, WIFEXITED(st) ? WEXITSTATUS(st) : 128 + WTERMSIG(st));
  return 0;
}
