#include <assert.h>
#include <string.h>
#include "dacp_endpoint.h"

int main(void) {
  char service[29];
  assert(dacp_service_name("aB", service, sizeof service));
  assert(strcmp(service, "iTunes_Ctrl_00000000000000AB") == 0);
  assert(dacp_same_id("aB", "00000000000000ab"));
  assert(!dacp_same_id("ab", "ac"));
  assert(!dacp_same_id("", "0"));
  assert(!dacp_service_name("00000000000000000", service, sizeof service));
  assert(!dacp_service_name("not-hex", service, sizeof service));
  assert(!dacp_service_name("ab", service, sizeof service - 1));
  assert(dacp_service_matches("iTunes_Ctrl_00000000000000AB", "ab"));
  assert(!dacp_service_matches("other_00000000000000AB", "ab"));
  assert(!dacp_service_matches("iTunes_Ctrl_00000000000000AC", "ab"));
  assert(dacp_unicast_ipv4("192.0.2.10", 3689));
  assert(!dacp_unicast_ipv4("192.0.2.10", 0));
  assert(!dacp_unicast_ipv4("0.0.0.0", 3689));
  assert(!dacp_unicast_ipv4("127.0.0.1", 3689));
  assert(!dacp_unicast_ipv4("224.0.0.1", 3689));
  assert(!dacp_unicast_ipv4("255.255.255.255", 3689));
  assert(!dacp_unicast_ipv4("::1", 3689));
  assert(!dacp_unicast_ipv4("not-an-address", 3689));
  return 0;
}
