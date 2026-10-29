#!/usr/bin/env python3
"""Compile the actual patched status/volume functions against no-audio stubs.

Run against a Shairport source tree after applying preplay-event-channel.patch.
This checks control before audio exists; it does not replace phone acceptance.
"""
from pathlib import Path
import subprocess
import sys
import tempfile

root = Path(sys.argv[1])
def function(path, signature):
    s = (root/path).read_text()
    start = s.index(signature)
    end = s.index('\n}', start) + 2
    return s[start:end]

status = function('dbus/dbus-service.c', 'static gboolean on_handle_get_control_status(')
volume = function('remote/remote.c', 'void station_control_set_volume(')
fixture = r'''
#define _GNU_SOURCE
#define CONFIG_AIRPLAY_2 1
#define CONFIG_METADATA 1
#include <assert.h>
#include <pthread.h>
#include <stdio.h>
#include <string.h>
#include <arpa/inet.h>
typedef int gboolean;
typedef void *gpointer;
typedef void ShairportSync;
typedef void GDBusMethodInvocation;
#define TRUE 1
#define FALSE 0
enum {ap_1, ap_2};
typedef struct {
 int airplay_type, event_channel_fd, own_airplay_volume_set;
 pthread_mutex_t event_sender_mutex, volume_control_mutex;
 char client_ip_string[INET6_ADDRSTRLEN];
 void *player_thread;
 double own_airplay_volume;
} rtsp_conn_info;
rtsp_conn_info *principal_conn, *station_event_conn;
pthread_rwlock_t principal_conn_lock=PTHREAD_RWLOCK_INITIALIZER;
pthread_rwlock_t station_event_conn_lock=PTHREAD_RWLOCK_INITIALIZER;
struct {double airplay_volume;} config;
int reported_available, reported_version, player_calls, metadata_calls;
char reported_client[INET6_ADDRSTRLEN];
void shairport_sync_complete_get_control_status(void *s, void *i, int v, int a, char *c) {
 (void)s;(void)i;reported_version=v;reported_available=a;
 snprintf(reported_client,sizeof(reported_client),"%s",c);
}
void player_volume(double v, rtsp_conn_info *c) {
 assert(c->player_thread != NULL);++player_calls;config.airplay_volume=v;
}
void send_ssnc_metadata(unsigned code, const char *p, size_t n, int blocking) {
 (void)code;(void)n;(void)blocking;
 assert(strstr(p,",0.00,0.00,0.00") != NULL);++metadata_calls;
}
'''
checks = r'''
int main(void) {
 rtsp_conn_info event={.airplay_type=ap_2,.event_channel_fd=5,
  .event_sender_mutex=PTHREAD_MUTEX_INITIALIZER,
  .volume_control_mutex=PTHREAD_MUTEX_INITIALIZER};
 strcpy(event.client_ip_string,"192.168.0.25");
 on_handle_get_control_status(NULL,NULL,NULL);
 assert(reported_version==1 && !reported_available && !reported_client[0]);
 station_event_conn=&event;
 on_handle_get_control_status(NULL,NULL,NULL);
 assert(reported_available && !strcmp(reported_client,"192.168.0.25"));
 station_control_set_volume(-24.0,&event);
 assert(player_calls==0 && metadata_calls==1);
 assert(config.airplay_volume==-24.0 && event.own_airplay_volume==-24.0);
 assert(event.own_airplay_volume_set==1);
 event.event_channel_fd=0;
 on_handle_get_control_status(NULL,NULL,NULL);
 assert(!reported_available && !reported_client[0]);
 event.event_channel_fd=5;
 principal_conn=&event;
 event.player_thread=&event;
 station_control_set_volume(-18.0,&event);
 assert(player_calls==1 && config.airplay_volume==-18.0);
 on_handle_get_control_status(NULL,NULL,NULL);
 assert(reported_available);
 principal_conn=NULL;station_event_conn=NULL;
 on_handle_get_control_status(NULL,NULL,NULL);
 assert(!reported_available && !reported_client[0]);
 puts("PREPLAY_STATUS_VOLUME_NO_AUDIO_PASS");
 return 0;
}
'''
with tempfile.TemporaryDirectory(prefix='preplay-unit-') as d:
    source=Path(d)/'test.c';binary=Path(d)/'test'
    source.write_text(fixture+'\n'+status+'\n'+volume+'\n'+checks)
    subprocess.run(['cc','-Wall','-Wextra','-Werror','-Wno-multichar','-pthread',str(source),'-o',str(binary)],check=True)
    subprocess.run([str(binary)],check=True)
