======================
Super Secret Kit (SSK)
======================

.. |release-badge| image:: https://github.com/Jv1i3nn/super-secret-kit/releases/latest/download/version.svg
   :alt: Dernière version publiée
   :target: https://github.com/Jv1i3nn/super-secret-kit/releases/latest

.. |go-badge| image:: https://img.shields.io/badge/Go-1.26%2B-00ADD8
   :alt: Go 1.26 ou plus récent
   :target: https://go.dev/

.. |license-badge| image:: https://img.shields.io/badge/Licence-MIT-green
   :alt: Licence MIT
   :target: https://opensource.org/license/mit

|release-badge| |go-badge| |license-badge|

Un outil CLI complet fournissant une gestion sécurisée des identifiants et une intégration transparente avec les services Jira, Tempo, Horoquartz et Arkevia.

.. contents:: Table des matières
   :depth: 3
   :local:

===============
Fonctionnalités
===============

🔐 **Sécurité**
--------------
- Chiffrement AES-256-CBC pour le stockage des identifiants
- Base de données locale sécurisée avec données sensibles chiffrées
- Stockage local des identifiants Jira, Tempo et Arkevia

⏱️ **Gestion du temps**
-----------------------
- Saisie automatisée du temps dans Jira/Tempo
- Filtrage intelligent (ignore les week-ends et jours fériés français)
- Vérification des entrées existantes pour éviter les doublons
- Sélection interactive des tickets
- Remplissage automatique des trous de saisie récents
- Opérations par lot avec plages de dates
- Recherche de tickets Jira par JQL et publication de commentaires Markdown
- Génération de contexte Jira pour briefing et inspection par agent de code
- Consultation des compteurs Horoquartz et déclaration de télétravail, CP, RTT et récupération

📄 **Gestion documentaire**
--------------------------
- Recherche et récupération de documents Arkevia
- Téléchargement PDF pour bulletins de paie et documents
- Sélection interactive de documents
- Ouverture automatique du fichier après téléchargement

🎯 **Expérience utilisateur**
----------------------------
- Workflows CLI interactifs
- Sortie colorisée dans le terminal
- Mode verbose pour le débogage
- Alias de commandes pour une utilisation rapide

============
Installation
============

Prérequis
---------

- Go 1.26.1 ou supérieur
- Make (pour compiler depuis les sources)
- Git

Depuis les sources
------------------

Cloner le dépôt et compiler :

.. code-block:: bash

   git clone https://github.com/Jv1i3nn/super-secret-kit.git
   cd super-secret-kit
   make build

Le binaire sera disponible dans ``bin/ssk``. Pour l’exécuter sans installation :

.. code-block:: bash

   ./bin/ssk --help

Installation système
--------------------

.. code-block:: bash

   make install

Cela installe le binaire dans ``$GOBIN`` si cette variable est définie, sinon dans ``$(go env GOPATH)/bin/ssk``. Ajoutez ce répertoire à votre ``PATH`` pour utiliser ``ssk`` directement.

Package Debian
--------------

Compiler et installer le package Debian :

.. code-block:: bash

   # Pour AMD64
   make VERSION=1.0.0 build-deb
   sudo dpkg -i bin/ssk_1.0.0_amd64.deb

   # Pour ARM64
   make VERSION=1.0.0 build-deb-arm64
   sudo dpkg -i bin/ssk_1.0.0_arm64.deb

   # Pour Raspberry Pi Zero (ARMv6 32 bits / armhf)
   make VERSION=1.0.0 build-deb-armv6
   sudo dpkg -i bin/ssk_1.0.0_armhf.deb

Package macOS
-------------

Compiler et installer le package macOS :

.. code-block:: bash

   # Pour Mac Intel
   make VERSION=1.0.0 build-pkg
   sudo installer -pkg bin/ssk-1.0.0-amd64.pkg -target /

   # Pour Mac Apple Silicon
   make VERSION=1.0.0 build-pkg-arm64
   sudo installer -pkg bin/ssk-1.0.0-arm64.pkg -target /

   # Binaire universel (Intel + Apple Silicon)
   make VERSION=1.0.0 build-pkg-universal
   sudo installer -pkg bin/ssk-1.0.0-universal.pkg -target /

Mise à jour
-----------

Pour une installation par package (``.deb`` ou ``.pkg``), télécharger et installer la dernière release GitHub :

.. code-block:: bash

   ssk update

La commande demande le mot de passe ``sudo`` pour installer le package. Une installation depuis les sources se met à jour avec ``git pull && make install``.

Compilation multi-plateformes
-----------------------------

Compiler pour plusieurs plateformes simultanément :

.. code-block:: bash

   make cross-compile

Les binaires seront disponibles dans ``bin/`` :

- ``ssk-linux-amd64``
- ``ssk-linux-armv6``
- ``ssk-darwin-amd64``
- ``ssk-darwin-arm64``

=============
Configuration
=============

Configuration initiale
----------------------

Exécuter la commande d'initialisation pour configurer SSK :

.. code-block:: bash

   ssk init

Cela crée le répertoire de configuration dans ``~/.ssk/`` avec :

- ``data.db`` - Base de données SQLite locale
- ``.passphrase`` - Clé locale utilisée pour le chiffrement

Le fichier ``config.yaml`` est optionnel. L'URL Jira est demandée à la première utilisation d'une commande Jira ou Tempo si elle n'est pas déjà configurée.

Chiffrement local
-----------------

Les nouveaux identifiants et sessions Horoquartz sont chiffrés avec AES-GCM au format ``ssk:v2:``. La clé utilise le nom de machine et la totalité de la passphrase ; toute modification des données chiffrées est détectée.

Le format historique AES-CBC compatible PHP reste lisible, sans réécriture automatique des données existantes. Ce format tronquait la clé à 32 octets : avec un nom de machine long, la passphrase pouvait être partiellement ou entièrement ignorée. Réenregistrer les identifiants et renouveler les sessions permet de les stocker au nouveau format.

Les anciennes versions de SSK et l'implémentation PHP ne peuvent pas lire le nouveau format. Conservez le fichier ``.passphrase`` et le nom de machine pour continuer à déchiffrer vos données.

URL Jira
--------

La première utilisation de ``ssk jira inspect``, ``ssk jira query``, ``ssk jira briefing``, ``ssk jira comment``, ``ssk logtime`` ou ``ssk autofill-time-gaps`` demande l'URL de votre site Jira et l'enregistre dans ``~/.ssk/.env`` :

.. code-block:: dotenv

   JIRA_API_URL="https://your-team.atlassian.net"

Les utilisations suivantes relisent ce fichier sans demander à nouveau l'URL. Les tokens et mots de passe restent chiffrés dans ``data.db`` ; ``.env`` contient les URL des services. Vous pouvez aussi créer ou modifier cette entrée directement. Le fichier est créé avec des permissions privées, et ses autres entrées sont conservées.

La variable d'environnement ``JIRA_API_URL`` a priorité sur ``jira_api_url`` dans ``config.yaml``, qui a elle-même priorité sur ``~/.ssk/.env``. Les URL Jira déjà configurées restent utilisables. Aucun site Jira n'est prédéfini.

Pour une utilisation automatisée sans entrée interactive, configurez l'URL à l'avance. La question initiale est affichée sur ``stderr`` pour préserver la sortie Markdown des commandes Jira.

URL Horoquartz
--------------

Les commandes ``ssk temporh`` (compteurs, télétravail, CP, RTT et récupération) demandent aussi leur URL à la première utilisation et l'enregistrent dans le même fichier ``~/.ssk/.env`` :

.. code-block:: dotenv

   HOROQUARTZ_URL="https://your-team.cloud-horoquartz.fr"

La variable d'environnement ``HOROQUARTZ_URL`` a priorité sur ``horoquartz_url`` dans ``config.yaml``, puis sur la valeur enregistrée dans ``.env``. Aucun site Horoquartz n'est prédéfini.

Fichier de configuration
------------------------

Le fichier de configuration optionnel se trouve dans ``~/.ssk/config.yaml``.

Vous pouvez spécifier un fichier de configuration personnalisé :

.. code-block:: bash

   ssk --config /chemin/vers/config.yaml <commande>

Multi-espace Jira (workspaces)
------------------------------

Par défaut, les commandes ``jira`` utilisent l'espace ``default`` et l'URL configurée ci-dessus. Pour inspecter un ticket sur un autre site Jira, utilisez le flag global ``--workspace`` :

.. code-block:: bash

   ssk jira inspect PROJ-123 --workspace autre-espace

Tous les espaces réutilisent les identifiants enregistrés sous ``jira``. Le même compte et le même token doivent donc avoir accès à chacun des sites configurés. ``--workspace`` change l’URL cible et ne sélectionne pas un autre token.

Les espaces supplémentaires sont définis dans ``jira_workspaces`` (URL du site Jira par nom d'espace). Pour en ajouter ou en modifier, éditez ``~/.ssk/config.yaml`` :

.. code-block:: yaml

   jira_workspaces:
     autre-espace: https://other-team.atlassian.net/

===========
Utilisation
===========

Afficher toutes les commandes disponibles :

.. code-block:: bash

   ssk

Obtenir de l'aide pour une commande spécifique :

.. code-block:: bash

   ssk <commande> --help

Commandes disponibles
---------------------

``init``
~~~~~~~~

Initialiser la configuration et la base de données SSK.

.. code-block:: bash

   ssk init

``credentials``
~~~~~~~~~~~~~~~

Gérer les identifiants pour les services Jira, Tempo et Arkevia.

.. code-block:: bash

   # Définir les identifiants Jira
   ssk credentials set jira <email> <api-token>

   # Définir les identifiants Tempo
   ssk credentials set jira_tempo <api-token>

   # Définir les identifiants Arkevia
   ssk credentials set arkevia <nom-utilisateur> <mot-de-passe>

   # Lister les identifiants stockés
   ssk credentials list

   # Supprimer les identifiants d'un service
   ssk credentials delete jira

``logtime`` (alias : ``tas_pas_timeshIte``)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Enregistrer les entrées de temps dans Jira/Tempo avec filtrage intelligent.

**Fonctionnalités :**

- Ignore automatiquement les week-ends
- Ignore automatiquement les jours fériés français
- Vérifie les entrées de temps existantes
- Sélection interactive des tickets

.. code-block:: bash

   # Saisie de temps interactive (sélectionner un ticket parmi les tickets assignés)
   ssk logtime

   # Enregistrer 8 heures aujourd'hui sur un ticket spécifique
   ssk logtime --ticket PROJ-123

   # Enregistrer 4 heures pour un jour spécifique
   ssk logtime --ticket PROJ-123 --day 2025-01-15 --hours 4

   # Enregistrer une semaine complète
   ssk logtime --ticket PROJ-123 --from 2025-01-01 --to 2025-01-05

   # Enregistrer une absence
   ssk logtime --off --day 2025-01-20

   # Enregistrer sans confirmation
   ssk logtime --ticket PROJ-123 --no-confirmation

**Options disponibles :**

- ``--ticket <clé>`` - Clé du ticket Jira (ex: PROJ-123)
- ``--day <date>`` - Jour unique au format AAAA-MM-JJ (par défaut : aujourd'hui)
- ``--from <date>`` - Date de début pour une plage (AAAA-MM-JJ)
- ``--to <date>`` - Date de fin pour une plage (AAAA-MM-JJ)
- ``--hours <n>`` - Heures à enregistrer (1-8, par défaut : 8)
- ``--off`` - Enregistrer sur TOT-1 (absence)
- ``--no-confirmation`` - Ignorer la confirmation
- ``--reset-credentials-jira`` - Réinitialiser les identifiants Jira
- ``--reset-credentials-tempo`` - Réinitialiser les identifiants Tempo

``autofill-time-gaps``
~~~~~~~~~~~~~~~~~~~~~~

Compléter automatiquement les jours récents qui n'atteignent pas 8h dans Tempo.

**Comportement :**

- Vérifie aujourd'hui et les 5 jours précédents
- Ignore les week-ends et jours fériés
- Ignore les jours déjà à 8h ou plus
- Choisit automatiquement le ticket Jira assigné le plus récemment modifié
- N'utilise un ticket que s'il existait déjà ce jour-là et n'était pas encore terminé
- Fonctionne sans interaction une fois l’URL Jira et les identifiants configurés

.. code-block:: bash

   # Exécution réelle
   ssk autofill-time-gaps

   # Prévisualisation sans créer de worklog Tempo
   ssk autofill-time-gaps --dry-run

**Options disponibles :**

- ``--dry-run`` - Afficher ce qui serait complété sans créer de worklogs Tempo

``jira query``
~~~~~~~~~~~~~~

Rechercher des tickets avec une requête JQL. Les résultats sont regroupés par statut et triés par clé de ticket.

.. code-block:: bash

   ssk jira query 'assignee = currentUser() AND resolution = Unresolved'
   ssk jira query 'project = PROJ ORDER BY updated DESC' --workspace autre-espace

``--workspace <nom>`` sélectionne un espace configuré dans ``jira_workspaces`` (par défaut : ``default``).

``jira briefing``
~~~~~~~~~~~~~~~~~

Générer un contexte Jira déterministe en Markdown pour un agent de code.

La commande récupère les tickets Jira assignés à l'utilisateur courant, non terminés, mis à jour dans les 30 derniers jours. Elle imprime uniquement le contexte source sur ``stdout`` : elle ne génère pas elle-même un briefing final en prose et n'appelle aucun LLM.

.. code-block:: bash

   # Générer le contexte Jira avec la limite par défaut de 10 tickets
   ssk jira briefing

   # Limiter le contexte aux 3 tickets les plus récemment mis à jour
   ssk jira briefing --max-tickets 3

   # Afficher les étapes de récupération sur stderr
   ssk --verbose jira briefing

   # Cibler un autre espace de travail Jira
   ssk jira briefing --workspace autre-espace

**Options disponibles :**

- ``--max-tickets <n>`` - Nombre maximum de tickets Jira à inclure (par défaut : 10)
- ``--workspace <nom>`` - Espace de travail Jira à cibler (par défaut : ``default``)

``jira inspect``
~~~~~~~~~~~~~~~~

Générer un contexte complet pour un ticket Jira et ses tickets liés.

La commande récupère le ticket demandé, puis les tickets liés par parent, sous-tâches et liens Jira. La recherche suit les relations en largeur avec une profondeur et une limite configurable. La sortie est un Markdown déterministe destiné aux agents de code.

.. code-block:: bash

   # Inspecter un ticket et ses relations directes
   ssk jira inspect PROJ-123

   # Limiter l'inspection aux relations directes
   ssk jira inspect PROJ-123 --depth 1

   # Augmenter la limite de tickets liés récupérés
   ssk jira inspect PROJ-123 --max-related 20

   # Afficher les étapes de récupération sur stderr
   ssk --verbose jira inspect PROJ-123

   # Cibler un autre espace de travail Jira
   ssk jira inspect PROJ-123 --workspace autre-espace

   # Télécharger les pièces jointes et médias intégrés
   ssk jira inspect PROJ-123 --download-attachments ./jira-artifacts/PROJ-123

**Options disponibles :**

- ``--depth <n>`` - Profondeur de parcours des relations Jira (par défaut : 2)
- ``--max-related <n>`` - Nombre maximum de tickets liés à récupérer, hors ticket racine (par défaut : 10)
- ``--download-attachments <répertoire>`` - Télécharge les pièces jointes Jira et indique les chemins locaux dans la sortie
- ``--workspace <nom>`` - Espace de travail Jira à cibler (par défaut : ``default``)

``jira comment``
~~~~~~~~~~~~~~~~

Publier un commentaire sur un ticket Jira.

Le commentaire est saisi en Markdown côté CLI, puis converti en Atlassian Document Format avant l'envoi à l'API Jira. Les éléments Markdown courants sont rendus dans l'interface Jira : titres, paragraphes, listes, citations, blocs de code, liens, gras, italique et code inline.

.. code-block:: bash

   # Publier un commentaire simple
   ssk jira comment PROJ-123 'hello world'

   # Publier un commentaire Markdown
   ssk jira comment PROJ-123 '## Analyse

   - Point important
   - Voir [la PR](https://github.com/example/repo/pull/123)

   ```go
   fmt.Println("hello")
   ```'

   # Publier le contenu d'un fichier Markdown
   ssk jira comment PROJ-123 "$(cat comment.md)"

   # Publier sur un ticket d'un autre espace de travail Jira
   ssk jira comment PROJ-123 'hello world' --workspace autre-espace

``arkevia`` (alias : ``payslip``, ``la_moula``)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Télécharger des documents depuis le service Arkevia.

.. code-block:: bash

   # Sélection interactive de documents
   ssk arkevia

   # Télécharger le dernier document
   ssk arkevia --latest
   ssk la_moula --latest

   # Télécharger tous les bulletins d'un mois (un mois peut en contenir plusieurs)
   ssk arkevia --year 2026 --month 5
   ssk payslip -y 2026 -m 5

   # Télécharger tous les documents d'une année entière
   ssk arkevia --year 2026

   # Lister les documents disponibles sans télécharger (non interactif)
   ssk arkevia --list
   ssk arkevia --list --rows 24

   # Télécharger vers un répertoire spécifique
   ssk arkevia --latest --output ~/Documents

**Options disponibles :**

- ``--output <dossier>`` ou ``-o <dossier>`` - Répertoire de destination (par défaut : ``~/.ssk/payslip``)
- ``--latest`` ou ``-l`` - Télécharger le dernier document
- ``--year <année>`` ou ``-y <année>`` - Télécharger tous les documents de cette année (ex : ``2026``)
- ``--month <mois>`` ou ``-m <mois>`` - Télécharger tous les documents de ce mois, 1-12 (nécessite ``--year``)
- ``--list`` - Lister les documents disponibles sans télécharger
- ``--rows <n>`` ou ``-n <n>`` - Nombre de documents récents à lister/proposer (par défaut : 10)

Un même mois peut contenir plusieurs bulletins (paie scindée en deux). Dans ce cas, ``--year`` et ``--month`` téléchargent **tous** les documents correspondants. ``--month`` seul renvoie une erreur demandant de préciser l'année.

``temporh``
~~~~~~~~~~~

Consulter les compteurs Horoquartz et déclarer du télétravail ou des absences. L’URL du service est demandée à la première utilisation si elle n’est pas configurée. L’authentification passe par le navigateur et la session est conservée dans ``~/.ssk/temporh``.

.. code-block:: bash

   # Consulter les compteurs
   ssk temporh --show-counters
   ssk temporh --show-counters --date 2026-12-31

   # Exporter les compteurs
   ssk temporh --format json
   ssk temporh --format csv > counters.csv

Options des compteurs :

- ``--show-counters`` ou ``-s`` - Afficher les compteurs (comportement par défaut)
- ``--date <date>`` ou ``-d`` - Date de référence (par défaut : 31 décembre de l’année courante)
- ``--format <format>`` ou ``-f`` - ``table``, ``json`` ou ``csv``
- ``--reset-credentials`` ou ``-r`` - Relancer l’authentification

Les sous-commandes suivantes enregistrent une demande dans Horoquartz :

.. code-block:: bash

   # Télétravail : journée, demi-journée ou période
   ssk temporh telework --date 2026-08-03
   ssk temporh telework --date 2026-08-03 --morning
   ssk temporh telework --start 2026-08-03 --end 2026-08-07

   # Congé payé, RTT et récupération
   ssk temporh cp --date 2026-08-03
   ssk temporh rtt --date 2026-08-03 --afternoon
   ssk temporh recup --date 2026-08-03

Options communes à ``telework``, ``cp``, ``rtt`` et ``recup`` :

- ``--date <date>`` ou ``-d`` - Une seule journée, au format AAAA-MM-JJ
- ``--start <date>`` ou ``-s`` et ``--end <date>`` ou ``-e`` - Une période
- ``--morning`` ou ``-m`` / ``--afternoon`` ou ``-a`` - Une demi-journée
- ``--start-morning``, ``--start-afternoon``, ``--end-morning``, ``--end-afternoon`` - Préciser les demi-journées aux limites d’une période
- ``--reset-credentials`` ou ``-r`` - Relancer l’authentification

``commands``
~~~~~~~~~~~~

Lister toutes les commandes disponibles avec leurs alias.

.. code-block:: bash

   ssk commands

``freedom``
~~~~~~~~~~~

Afficher le temps restant avant la date de la liberté (30 juin 2027 à 17h00).
Par défaut, la commande affiche le décompte détaillé complet.

.. code-block:: bash

   # Affichage complet par défaut (avec statistiques calendaires et ouvrées)
   ssk freedom

   # Filtrer par jours ouvrés
   ssk freedom --workdays
   ssk freedom -w

   # Combiner --workdays (-w) avec toutes les autres unités (heures, minutes, secondes)
   ssk freedom -w -H       # Heures ouvrées uniquement
   ssk freedom -w -m       # Minutes ouvrées uniquement
   ssk freedom -w -s       # Secondes ouvrées uniquement

   # Sortie brute (pour les scripts)
   ssk freedom -w -H --raw

**Options disponibles :**

- ``--workdays`` ou ``-w`` - Filtrer le calcul sur le temps ouvré (combinable avec toutes les unités)
- ``--format <type>`` ou ``-f <type>`` - Type de sortie (``jour``, ``ouvrés``, ``heure``, ``minutes``, ``secondes``, ``full``)
- ``--days`` ou ``-d`` - Shortcut pour affichage en jours
- ``--hours`` ou ``-H`` - Shortcut pour affichage en heures
- ``--minutes`` ou ``-m`` - Shortcut pour affichage en minutes
- ``--seconds`` ou ``-s`` - Shortcut pour affichage en secondes
- ``--raw`` ou ``-r`` - Afficher uniquement la valeur numérique brute

Options globales
----------------

``--config <fichier>``
~~~~~~~~~~~~~~~~~~~~~~

Spécifier un chemin de fichier de configuration personnalisé.

.. code-block:: bash

   ssk --config ~/.ssk/custom-config.yaml <commande>

``--verbose`` ou ``-v``
~~~~~~~~~~~~~~~~~~~~~~~

Activer la sortie détaillée pour le débogage.

.. code-block:: bash

   ssk -v <commande>

``--version``
~~~~~~~~~~~~~

Afficher la version actuelle.

.. code-block:: bash

   ssk --version

=============
Développement
=============

Configuration de l'environnement de développement
-------------------------------------------------

1. Cloner le dépôt :

   .. code-block:: bash

      git clone https://github.com/Jv1i3nn/super-secret-kit.git
      cd super-secret-kit

2. Installer les dépendances :

   .. code-block:: bash

      make deps

3. Installer les outils de développement :

   .. code-block:: bash

      # Installer golangci-lint
      go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

      # Installer air pour le rechargement automatique (optionnel)
      go install github.com/cosmtrek/air@latest

Compilation
-----------

Compiler le projet :

.. code-block:: bash

   make build

Compiler et exécuter :

.. code-block:: bash

   make run

Mode développement avec rechargement automatique :

.. code-block:: bash

   make dev

Tests
-----

Exécuter les tests :

.. code-block:: bash

   make test

Exécuter les tests courts (excluant les tests longs) :

.. code-block:: bash

   make test-short

Exécuter les tests avec détection de race conditions :

.. code-block:: bash

   make test-race

Générer un rapport de couverture :

.. code-block:: bash

   make test-coverage

Visualiser la couverture dans le navigateur :

.. code-block:: bash

   make test-coverage-report

Exécuter les benchmarks :

.. code-block:: bash

   make test-bench

Intégration continue
--------------------

Le workflow ``.github/workflows/ci.yml`` se déclenche à chaque push de branche, sur les pull requests et manuellement depuis GitHub Actions.

Il vérifie :

- Le formatage et le lint avec golangci-lint v2.14.0 et ``.golangci.yml``
- La cohérence et l’intégrité des dépendances Go
- ``go vet``
- Les tests avec détection des courses concurrentes et couverture sur Linux et macOS
- La compilation du binaire sur Linux et macOS

Les rapports de couverture sont disponibles dans les artefacts GitHub Actions pendant 7 jours. La version de Go utilisée est celle de ``go.mod``.

Pour reproduire les contrôles localement :

.. code-block:: bash

   make lint
   go mod verify
   go mod tidy -diff
   make vet
   go test -race -count=1 -coverprofile=coverage.out ./...
   make VERSION=ci build

Release
-------

Les releases sont automatisées par GitHub Actions via ``.github/workflows/release.yml``.

Le workflow se déclenche lors du push d’un tag ``v*`` ou manuellement depuis GitHub Actions en indiquant un tag existant. Il transmet le nom du tag (sans le préfixe ``v``) aux builds, construit les packages Debian et macOS, génère un badge de version SVG, puis crée la GitHub Release avec les artefacts attachés.

Publier une release :

.. code-block:: bash

   # Vérifier localement avant publication
   make all

   # Créer et pousser le tag
   git tag v2.0.2
   git push origin v2.0.2

Artefacts produits automatiquement :

- ``ssk_<version>_amd64.deb``
- ``ssk_<version>_arm64.deb``
- ``ssk_<version>_armhf.deb`` (ARMv6 32 bits, Raspberry Pi Zero)
- ``ssk-<version>-amd64.pkg``
- ``ssk-<version>-arm64.pkg``
- ``ssk-<version>-universal.pkg``

Les builds locaux dérivent leur version de Git : un checkout sur un tag affiche ce tag, tandis qu'un commit de développement inclut sa distance au dernier tag et son identifiant court. Pour forcer une version, par exemple dans une intégration externe, utiliser ``make VERSION=2.0.2 install``.

==========================
Automatisation Quotidienne
==========================

Vous pouvez planifier ``ssk autofill-time-gaps`` tous les jours à midi avec ``cron`` sur Linux.

Prérequis
---------

- Le binaire ``ssk`` doit être installé sur la machine
- Les identifiants ``jira`` et ``jira_tempo`` doivent déjà être enregistrés
- Le répertoire ``~/.ssk/`` doit déjà exister sur cette machine
- L’URL Jira doit être configurée dans ``~/.ssk/.env``, ``config.yaml`` ou ``JIRA_API_URL``

Vérifiez d'abord manuellement :

.. code-block:: bash

   ssk autofill-time-gaps --dry-run

Linux avec cron
---------------

Éditer la crontab de l'utilisateur :

.. code-block:: bash

   crontab -e

Ajouter cette ligne pour une exécution tous les jours à ``12:00`` :

.. code-block:: cron

   0 12 * * * /home/<user>/go/bin/ssk autofill-time-gaps >> /home/<user>/.ssk/autofill-time-gaps.log 2>&1

Remarques :

- Remplacez ``/home/<user>/go/bin/ssk`` par le chemin réel du binaire
- Si vous utilisez un fichier de config personnalisé, ajoutez ``--config /chemin/config.yaml``

Conseils
--------

- Commencez avec ``--dry-run`` pendant un ou deux jours avant d'activer l'exécution réelle
- Vérifiez régulièrement le fichier de log

Qualité du code
---------------

Formater le code :

.. code-block:: bash

   make fmt

Exécuter le linter :

.. code-block:: bash

   make lint

Exécuter ``go vet`` :

.. code-block:: bash

   make vet

Exécuter toutes les vérifications de qualité :

.. code-block:: bash

   make all

Cela exécute : clean, deps, fmt, vet, test et build.

Structure du projet
-------------------

.. code-block:: text

   super-secret-kit/
   ├── .github/workflows/      # Tags et releases GitHub Actions
   ├── cmd/                   # Commandes CLI Cobra
   │   ├── root.go            # Commande racine et configuration globale
   │   ├── service_config.go  # Saisie initiale des URL des services
   │   ├── init.go            # Initialisation locale
   │   ├── credentials.go     # Identifiants chiffrés
   │   ├── logtime.go         # Saisie du temps Tempo
   │   ├── autofill_time_gaps.go # Remplissage automatique Tempo
   │   ├── jira.go            # Espaces et client Jira
   │   ├── jira_query.go      # Recherche JQL
   │   ├── briefing.go        # Contexte des tickets assignés
   │   ├── jira_inspect.go    # Inspection d’un ticket et de ses relations
   │   ├── jira_comment.go    # Commentaires Markdown
   │   ├── temporh.go         # Compteurs Horoquartz
   │   ├── temporh_telework.go # Télétravail
   │   ├── temporh_requests.go # CP, RTT et récupération
   │   ├── arkevia.go         # Documents Arkevia
   │   ├── freedom.go         # Décompte calendaire et ouvré
   │   └── list.go            # Liste des commandes et alias
   ├── internal/
   │   ├── config/            # Configuration YAML, environnement et .env
   │   ├── database/          # SQLite et migrations
   │   ├── models/            # Modèles de données
   │   ├── repository/        # Accès aux données
   │   ├── services/          # Jira, Tempo, Horoquartz, Arkevia et chiffrement
   │   ├── utils/             # Utilitaires système et TOTP
   │   ├── version/           # Version injectée à la compilation
   │   └── web/static/        # Ressources web statiques
   ├── scripts/
   │   ├── postinst           # Post-installation Debian
   │   └── postinstall.macos  # Post-installation macOS
   ├── build/                 # Artefacts des packages (générés)
   ├── bin/                   # Binaires et packages (générés)
   ├── main.go                # Point d’entrée
   ├── go.mod                 # Module et dépendances Go
   ├── go.sum                 # Sommes de contrôle
   ├── Makefile               # Compilation, tests et installation
   └── README.rst             # Ce fichier

Dépendances
-----------

Dépendances principales :
~~~~~~~~~~~~~~~~~~~~~~~~~

- ``github.com/spf13/cobra`` - Framework CLI
- ``github.com/spf13/viper`` - Gestion de la configuration
- ``gorm.io/gorm`` - ORM pour les opérations base de données
- ``gorm.io/driver/sqlite`` - Driver SQLite
- ``modernc.org/sqlite`` - Implémentation SQLite pure Go

Voir ``go.mod`` pour la liste complète des dépendances.

=====
Alias
=====

SSK supporte les alias de commandes pour une utilisation plus rapide :

- ``logtime`` → ``tas_pas_timeshIte``
- ``arkevia`` → ``payslip``, ``la_moula``
- ``temporh telework`` → ``tt``, ``add-telework``
- ``temporh cp`` → ``conge``, ``conges``, ``add-cp``
- ``temporh rtt`` → ``add-rtt``
- ``temporh recup`` → ``recovery``, ``add-recup``

=============
Style de code
=============

- Suivre les conventions Go standard
- Exécuter ``make fmt`` avant de committer
- S'assurer que ``make lint`` passe sans avertissements
- Ajouter des tests pour les nouvelles fonctionnalités
- Mettre à jour la documentation selon les besoins

=======
Support
=======

Pour les problèmes, questions ou contributions :

- **Issues** : https://github.com/Jv1i3nn/super-secret-kit/issues
- **Discussions** : https://github.com/Jv1i3nn/super-secret-kit/discussions
