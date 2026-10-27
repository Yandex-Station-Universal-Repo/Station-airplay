#include <assert.h>
#include <limits.h>
#include "stream-gain.h"
int main(void) {
  stream_gain_lease s = {100,0};
  assert(stream_gain_fixed(&s, 1)==65536);
  assert(stream_gain_update(&s,20,4500,100));
  assert(stream_gain_fixed(&s,101)==13107);
  assert(stream_gain_fixed(&s,4599)==13107);
  assert(stream_gain_fixed(&s,4600)==65536);
  assert(!stream_gain_update(&s,101,100,1));
  assert(!stream_gain_update(&s,20,0,1));
  assert(!stream_gain_update(&s,20,5001,1));
  assert(stream_gain_update(&s,0,100,1));
  assert(stream_gain_fixed(&s,2)==0);
  assert(stream_gain_update(&s,100,0,2));
  assert(stream_gain_fixed(&s,2)==65536);
  int64_t sample=INT32_MIN;
  assert(sample*13107/65536 > INT32_MIN);
  return 0;
}
