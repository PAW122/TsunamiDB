# /servers/core

główny package serwera

po włączeniu serwera:
wczytaj config
wczytaj mapy z /db
połącz do sieci
brodcast: {server connected to network}
async start_network_listener()
test_ping: {connection_check} -> jeżeli są inne serwery to powinny potwierdzić połączenie
