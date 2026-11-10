// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:ptpatronus/api/agent.dart';

void main() {
  test('AgentApi can chat again after cancel', () async {
    var clientsCreated = 0;
    http.Client clientFactory() {
      clientsCreated++;
      return MockClient((request) async {
        expect(request.url.path, '/api/v1/agent/chat');
        return http.Response(
          'event: conversation\r\ndata: {"id":1}\r\n\r\n'
          'event: token\r\ndata: {"content":"ok"}\r\n\r\n'
          'event: done\r\ndata: {}\r\n\r\n',
          200,
          headers: {'content-type': 'text/event-stream'},
        );
      });
    }

    final api = AgentApi(
      baseUrl: 'http://assistant.test',
      clientFactory: clientFactory,
    );

    api.cancel();
    final first = await api.chat('first', null).toList();
    api.cancel();
    final second = await api.chat('second', 1).toList();

    expect(first.map((event) => event.event), [
      'conversation',
      'token',
      'done',
    ]);
    expect(second.map((event) => event.event), [
      'conversation',
      'token',
      'done',
    ]);
    expect(clientsCreated, 2);
    api.cancel();
  });
}
