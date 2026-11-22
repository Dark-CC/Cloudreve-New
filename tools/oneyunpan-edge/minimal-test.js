// Paste this into a fresh Snippet first. If it saves and replies "ok" on
// https://upload.asnndi.com/, the Snippet feature itself works and the full
// script can be pasted over it. If it fails, the problem is the Zone/plan and
// not our code.
export default {
  async fetch() {
    return new Response("ok", {
      status: 200,
      headers: { "content-type": "text/plain", "access-control-allow-origin": "*" },
    });
  },
};
